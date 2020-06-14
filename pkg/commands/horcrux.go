package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type HorcruxHeader struct {
	OriginalFilename string `json:"originalFilename"`
	Timestamp        int64  `json:"timestamp"`
	Index            int    `json:"index"`
	Total            int    `json:"total"`
	Threshold        int    `json:"threshold"`
	KeyFragment      []byte `json:"keyFragment"`
}

type Horcrux struct {
	path   string
	header HorcruxHeader
	file   *os.File
}

// returns a horcrux with its header parsed, and it's file's read pointer
// right after the header.
func NewHorcrux(path string) (*Horcrux, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	header, err := GetHeaderFromHorcruxFile(file)
	if err != nil {
		return nil, err
	}

	return &Horcrux{
		path:   path,
		file:   file,
		header: *header,
	}, nil
}

// this function gets the header from the horcrux file and ensures that we leave
// the file with its read pointer at the start of the encrypted content
// so that we can later directly read from that point
// yes this is a side effect, no I'm not proud of it.
func GetHeaderFromHorcruxFile(file *os.File) (*HorcruxHeader, error) {
	currentHeader := &HorcruxHeader{}
	scanner := bufio.NewScanner(file)
	bytesBeforeBody := 0
	for scanner.Scan() {
		line := scanner.Text()
		bytesBeforeBody += len(scanner.Bytes()) + 1
		if line == "-- HEADER --" {
			scanner.Scan()
			bytesBeforeBody += len(scanner.Bytes()) + 1
			headerLine := scanner.Bytes()
			if err := json.Unmarshal(headerLine, currentHeader); err != nil {
				return nil, err
			}

			scanner.Scan() // one more to get past the body line
			bytesBeforeBody += len(scanner.Bytes()) + 1
			break
		}
	}
	if _, err := file.Seek(int64(bytesBeforeBody), io.SeekStart); err != nil {
		return nil, err
	}

	if currentHeader == nil {
		return nil, errors.New("could not find header in horcrux file")
	}
	return currentHeader, nil
}

func (h *Horcrux) GetHeader() HorcruxHeader {
	return h.header
}

func (h *Horcrux) GetPath() string {
	return h.path
}

func (h *Horcrux) GetFile() *os.File {
	return h.file
}
