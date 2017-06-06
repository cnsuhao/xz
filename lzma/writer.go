// Copyright 2014-2017 Ulrich Kunitz. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

import (
	"bufio"
	"errors"
	"io"
)

// MinDictCap and MaxDictCap provide the range of supported dictionary
// capacities.
const (
	MinDictCap = 1 << 12
	MaxDictCap = 1<<32 - 1
)

// WriterConfig defines the configuration parameter for a writer.
type WriterConfig struct {
	// Properties for the encoding. If the it is nil the value
	// {LC: 3, LP: 0, PB: 2} will be chosen.
	Properties *Properties
	// The capacity of the dictionary. If DictCap is zero, the value
	// 8 MiB will be chosen.
	DictCap int
	// Size of the lookahead buffer; value 0 indicates default size
	// 4096
	BufSize int
	// Match finder algorithm
	MatchFinder MatchFinder
	// SizeInHeader indicates that the header will contain an
	// explicit size.
	SizeInHeader bool
	// Size of the data to be encoded. A positive value will imply
	// than an explicit size will be set in the header.
	Size int64
	// EOSMarker requests whether the EOSMarker needs to be written.
	// If no explicit size is been given the EOSMarker will be
	// set automatically.
	EOSMarker bool
	NiceLen   int
	Depth     int
}

// fill converts zero-value fields to their explicit default values.
func (c *WriterConfig) fill() {
	if c.Properties == nil {
		c.Properties = &Properties{LC: 3, LP: 0, PB: 2}
	}
	if c.DictCap == 0 {
		c.DictCap = 8 * 1024 * 1024
	}
	if c.BufSize == 0 {
		c.BufSize = 4096
	}
	if c.Size > 0 {
		c.SizeInHeader = true
	}
	if !c.SizeInHeader {
		c.EOSMarker = true
	}
	if c.MatchFinder == 0 {
		c.MatchFinder = HashChain4
	}
	if c.NiceLen == 0 {
		c.NiceLen = 64
	}
}

// verify checks WriterConfig for errors. Verify will replace zero
// values with default values.
func (c *WriterConfig) verify() error {
	c.fill()
	var err error
	if c == nil {
		return errors.New("lzma: WriterConfig is nil")
	}
	if c.Properties == nil {
		return errors.New("lzma: WriterConfig has no Properties set")
	}
	if err = c.Properties.verify(); err != nil {
		return err
	}
	if !(MinDictCap <= c.DictCap && int64(c.DictCap) <= MaxDictCap) {
		return errors.New("lzma: dictionary capacity is out of range")
	}
	if !(maxMatchLen <= c.BufSize) {
		return errors.New("lzma: lookahead buffer size too small")
	}
	if c.SizeInHeader {
		if c.Size < 0 {
			return errors.New("lzma: negative size not supported")
		}
	} else if !c.EOSMarker {
		return errors.New("lzma: EOS marker is required")
	}
	if err = c.MatchFinder.verify(); err != nil {
		return err
	}
	if !(4 <= c.NiceLen && c.NiceLen <= maxMatchLen) {
		return errors.New("lzma: NiceLen out of range")
	}
	if c.Depth < 0 {
		return errors.New("lzma: Depth is negative")
	}
	return nil
}

// header returns the header structure for this configuration.
func (c *WriterConfig) header() header {
	h := header{
		properties: *c.Properties,
		dictCap:    c.DictCap,
		size:       -1,
	}
	if c.SizeInHeader {
		h.size = c.Size
	}
	return h
}

// Writer writes an LZMA stream in the classic format.
type Writer struct {
	h   header
	bw  io.ByteWriter
	buf *bufio.Writer
	e   *encoder
}

// NewWriterCfg creates a new LZMA writer for the classic format. The
// method will write the header to the underlying stream.
func NewWriterCfg(lzma io.Writer, cfg WriterConfig) (w *Writer, err error) {
	if err = cfg.verify(); err != nil {
		return nil, err
	}
	w = &Writer{h: cfg.header()}

	var ok bool
	w.bw, ok = lzma.(io.ByteWriter)
	if !ok {
		w.buf = bufio.NewWriter(lzma)
		w.bw = w.buf
	}
	state := newState(w.h.properties)
	m, err := cfg.MatchFinder.new(w.h.dictCap, cfg.BufSize, cfg.NiceLen,
		cfg.Depth)
	if err != nil {
		return nil, err
	}
	var flags encoderFlags
	if cfg.EOSMarker {
		flags = eosMarker
	}
	if w.e, err = newEncoder(w.bw, state, m, flags); err != nil {
		return nil, err
	}

	if err = w.writeHeader(); err != nil {
		return nil, err
	}
	return w, nil
}

// NewWriter creates a new LZMA writer using the classic format. The
// function writes the header to the underlying stream.
func NewWriter(lzma io.Writer) (w *Writer, err error) {
	return NewWriterCfg(lzma, WriterConfig{})
}

// writeHeader writes the LZMA header into the stream.
func (w *Writer) writeHeader() error {
	data, err := w.h.marshalBinary()
	if err != nil {
		return err
	}
	_, err = w.bw.(io.Writer).Write(data)
	return err
}

// Write puts data into the Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	if w.h.size >= 0 {
		m := w.h.size
		m -= w.e.Compressed() + int64(w.e.dict.Buffered())
		if m < 0 {
			m = 0
		}
		if m < int64(len(p)) {
			p = p[:m]
			err = ErrNoSpace
		}
	}
	var werr error
	if n, werr = w.e.Write(p); werr != nil {
		err = werr
	}
	return n, err
}

// Close closes the writer stream. It ensures that all data from the
// buffer will be compressed and the LZMA stream will be finished.
func (w *Writer) Close() error {
	if w.h.size >= 0 {
		n := w.e.Compressed() + int64(w.e.dict.Buffered())
		if n != w.h.size {
			return errSize
		}
	}
	err := w.e.Close()
	if w.buf != nil {
		ferr := w.buf.Flush()
		if err == nil {
			err = ferr
		}
	}
	return err
}
