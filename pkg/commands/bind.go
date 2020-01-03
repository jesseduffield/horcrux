package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jesseduffield/horcrux/pkg/multiplexing"
	"github.com/jesseduffield/horcrux/pkg/shamir"
)

func Bind(dir string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	filenames := []string{}
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".horcrux" {
			filenames = append(filenames, file.Name())
		}
	}

	headers := []horcruxHeader{}
	horcruxFiles := []*os.File{}

	for _, filename := range filenames {
		file, err := os.Open(filename)
		defer file.Close()
		if err != nil {
			return err
		}

		currentHeader, err := getHeaderFromHorcruxFile(file)
		if err != nil {
			return err
		}

		for _, header := range headers {
			if header.Index == currentHeader.Index {
				// we've already obtained this horcrux so we'll skip this instance
				continue
			}
		}

		if len(headers) > 0 && (currentHeader.OriginalFilename != headers[0].OriginalFilename || currentHeader.Timestamp != headers[0].Timestamp) {
			return errors.New("All horcruxes in the given directory must have the same original filename and timestamp.")
		}

		headers = append(headers, *currentHeader)
		horcruxFiles = append(horcruxFiles, file)
	}

	if len(headers) == 0 {
		return errors.New("No horcruxes in directory")
	} else if len(headers) < headers[0].Threshold {
		return errors.New(fmt.Sprintf("You do not have all the required horcruxes. There are %d required to resurrect the original file. You only have %d", headers[0].Threshold, len(headers)))
	}

	keyFragments := make([][]byte, len(headers))
	for i := range keyFragments {
		keyFragments[i] = headers[i].KeyFragment
	}

	key, err := shamir.Combine(keyFragments)
	if err != nil {
		return err
	}

	var fileReader io.Reader
	if headers[0].Total == headers[0].Threshold {
		// sort by index
		orderedHorcruxFiles := make([]*os.File, len(horcruxFiles))
		for i, h := range horcruxFiles {
			orderedHorcruxFiles[headers[i].Index-1] = h
		}

		fileReader = &multiplexing.Multiplexer{Readers: orderedHorcruxFiles}
	} else {
		fileReader = horcruxFiles[0] // arbitrarily read from the first horcrux: they all contain the same contents
	}

	reader := cryptoReader(fileReader, key)

	newFilename := headers[0].OriginalFilename
	if fileExists(newFilename) {
		newFilename = prompt("A file already exists named '%s'. Enter new file name: ", newFilename)
	}

	_ = os.Truncate(newFilename, 0)

	newFile, err := os.OpenFile(newFilename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, reader)
	if err != nil {
		return err
	}

	return err
}

// this function gets the header from the horcrux file and ensures that we leave
// the file with its read pointer at the start of the encrypted content
// so that we can later directly read from that point
// yes this is a side effect, no I'm not proud of it.
func getHeaderFromHorcruxFile(file *os.File) (*horcruxHeader, error) {
	currentHeader := &horcruxHeader{}
	scanner := bufio.NewScanner(file)
	bytesBeforeBody := 0
	for scanner.Scan() {
		line := scanner.Text()
		bytesBeforeBody += len(scanner.Bytes()) + 1
		if line == "-- HEADER --" {
			scanner.Scan()
			bytesBeforeBody += len(scanner.Bytes()) + 1
			headerLine := scanner.Bytes()
			json.Unmarshal(headerLine, currentHeader)
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
