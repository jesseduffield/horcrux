package multiplexing

// when we set the threshold to the number of horcruxes, we can save space by
// dividing the encrypted contents equally across each horcrux.
// This file contains a multiplexer/Demultiplexer for reading/writing with
// multiplexed content

import "os"

const BYTE_QUOTA = 100

func min(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

type Demultiplexer struct {
	Writers      []*os.File
	writerIndex  int
	bytesWritten int
}

func (d *Demultiplexer) nextWriter() {
	d.writerIndex++
	if d.writerIndex > len(d.Writers)-1 {
		d.writerIndex = 0
	}
	d.bytesWritten = 0
}

func (d *Demultiplexer) Write(p []byte) (int, error) {
	totalN := 0
	for totalN < len(p) {
		remainingBytes := len(p) - totalN
		remainingBytesForWriter := BYTE_QUOTA - d.bytesWritten
		n, err := d.Writers[d.writerIndex].Write(p[totalN : totalN+min(remainingBytesForWriter, remainingBytes)])
		d.bytesWritten += n
		totalN += n
		if err != nil {
			return totalN, err
		}
		if remainingBytesForWriter-n <= 0 {
			d.nextWriter()
		}
	}

	return totalN, nil
}

type Multiplexer struct {
	Readers     []*os.File
	readerIndex int
	bytesRead   int
}

func (m *Multiplexer) nextReader() {
	m.readerIndex++
	if m.readerIndex > len(m.Readers)-1 {
		m.readerIndex = 0
	}
	m.bytesRead = 0
}

func (m *Multiplexer) Read(p []byte) (int, error) {
	totalN := 0
	for totalN < len(p) {
		remainingBytes := len(p) - totalN
		remainingBytesForReader := BYTE_QUOTA - m.bytesRead
		buf := make([]byte, min(remainingBytes, remainingBytesForReader))
		n, err := m.Readers[m.readerIndex].Read(buf)
		p = append(p[0:totalN], buf[0:n]...)
		totalN += n
		m.bytesRead += n
		if err != nil {
			return totalN, err
		}
		if remainingBytesForReader-n <= 0 {
			m.nextReader()
		}
	}

	return totalN, nil
}
