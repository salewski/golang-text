// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package transform provides reader and writer wrappers that transform the
// bytes passing through. Example transformations, provided by other packages,
// include text collation, normalization and conversion between character sets.
package transform

// TODO: can a Transformer be reset? How (Reset call, implied by convention)? If
// explicit, as part of the Transformer interface or a different, optional
// interface?

import (
	"errors"
	"io"
)

var (
	// ErrShortDst means that the destination buffer was too short to
	// receive all of the transformed bytes.
	ErrShortDst = errors.New("transform: short destination buffer")

	// ErrShortSrc means that the source buffer has insufficient data to
	// complete the transformation.
	ErrShortSrc = errors.New("transform: short source buffer")

	// errInconsistentByteCount means that Transform returned success (nil
	// error) but also returned nSrc inconsistent with the src argument.
	errInconsistentByteCount = errors.New("transform: inconsistent byte count returned")
)

// Transformer transforms bytes.
type Transformer interface {
	// Transform writes to dst the transformed bytes read from src, and
	// returns the number of dst bytes written and src bytes read. The
	// atEOF argument tells whether src represents the last bytes of the
	// input.
	//
	// Callers should always process the nDst bytes produced and account
	// for the nSrc bytes consumed before considering the error err.
	//
	// A nil error means that all of the transformed bytes (whether freshly
	// transformed from src or left over from previous Transform calls)
	// were written to dst. A nil error can be returned regardless of
	// whether atEOF is true. If err is nil then nSrc must equal len(src);
	// the converse is not necessarily true.
	//
	// ErrShortDst means that dst was too short to receive all of the
	// transformed bytes. ErrShortSrc means that src had insufficient data
	// to complete the transformation. If both conditions apply, then
	// either error may be returned. Other than the error conditions listed
	// here, implementations are free to report other errors that arise.
	Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
}

// Reader wraps another io.Reader by transforming the bytes read.
type Reader struct {
	r   io.Reader
	t   Transformer
	err error

	// dst[dst0:dst1] contains bytes that have been transformed by t but
	// not yet copied out via Read.
	dst        []byte
	dst0, dst1 int

	// src[src0:src1] contains bytes that have been read from r but not
	// yet transformed through t.
	src        []byte
	src0, src1 int

	// transformComplete is whether the transformation is complete,
	// regardless of whether or not it was successful.
	transformComplete bool
}

// NewReader returns a new Reader that wraps r by transforming the bytes read
// via t.
func NewReader(r io.Reader, t Transformer) *Reader {
	return &Reader{
		r:   r,
		t:   t,
		dst: make([]byte, 4096),
		src: make([]byte, 4096),
	}
}

// Read implements the io.Reader interface.
func (r *Reader) Read(p []byte) (int, error) {
	n, err := 0, error(nil)
	for {
		// Copy out any transformed bytes and return the final error if we are done.
		if r.dst0 != r.dst1 {
			n = copy(p, r.dst[r.dst0:r.dst1])
			r.dst0 += n
			if r.dst0 == r.dst1 && r.transformComplete {
				return n, r.err
			}
			return n, nil
		} else if r.transformComplete {
			return 0, r.err
		}

		// Try to transform some source bytes, or to flush the transformer if we
		// are out of source bytes. We do this even if r.r.Read returned an error.
		// As the io.Reader documentation says, "process the n > 0 bytes returned
		// before considering the error".
		if r.src0 != r.src1 || r.err != nil {
			r.dst0 = 0
			r.dst1, n, err = r.t.Transform(r.dst, r.src[r.src0:r.src1], r.err == io.EOF)
			r.src0 += n

			switch {
			case err == nil:
				if r.src0 != r.src1 {
					r.err = errInconsistentByteCount
				}
				// The Transform call was successful; we are complete if we
				// cannot read more bytes into src.
				r.transformComplete = r.err != nil
				continue
			case err == ErrShortDst && r.dst1 != 0:
				// Make room in dst by copying out, and try again.
				continue
			case err == ErrShortSrc && r.src1-r.src0 != len(r.src) && r.err == nil:
				// Read more bytes into src via the code below, and try again.
			default:
				r.transformComplete = true
				// The reader error (r.err) takes precedence over the
				// transformer error (err) unless r.err is nil or io.EOF.
				if r.err == nil || r.err == io.EOF {
					r.err = err
				}
				continue
			}
		}

		// Move any untransformed source bytes to the start of the buffer
		// and read more bytes.
		if r.src0 != 0 {
			r.src0, r.src1 = 0, copy(r.src, r.src[r.src0:r.src1])
		}
		n, r.err = r.r.Read(r.src[r.src1:])
		r.src1 += n
	}
}

// TODO: implement ReadByte (and ReadRune??).

// TODO: type Writer.
