package commands

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jesseduffield/horcrux/pkg/multiplexing"
	"github.com/jesseduffield/horcrux/pkg/shamir"
)

func Split(path string) error {
	total, threshold, err := obtainTotalAndThreshold()

	key, err := generateKey()
	if err != nil {
		return err
	}

	keyFragments, err := shamir.Split(key, total, threshold)
	if err != nil {
		return err
	}

	timestamp := time.Now().Unix()

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	originalFilename := filepath.Base(path)

	horcruxFiles := make([]*os.File, total)

	for i := range horcruxFiles {
		index := i + 1

		headerBytes, err := json.Marshal(&horcruxHeader{
			OriginalFilename: originalFilename,
			Timestamp:        timestamp,
			Index:            index,
			Total:            total,
			KeyFragment:      keyFragments[i],
			Threshold:        threshold,
		})
		if err != nil {
			return err
		}

		originalFilenameWithoutExt := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
		horcruxFilename := fmt.Sprintf("%s_%d_of_%d.horcrux", originalFilenameWithoutExt, index, total)
		fmt.Printf("creating %s\n", horcruxFilename)

		// clearing file in case it already existed
		_ = os.Truncate(horcruxFilename, 0)

		horcruxFile, err := os.OpenFile(horcruxFilename, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		defer horcruxFile.Close()

		horcruxFile.WriteString(header(index, total, headerBytes))

		horcruxFiles[i] = horcruxFile
	}

	// wrap file reader in an encryption stream
	var fileReader io.Reader = file
	reader := cryptoReader(fileReader, key)

	var writer io.Writer
	if threshold == total {
		// because we need all horcruxes to reconstitute the original file,
		// we'll use a multiplexer to divide the encrypted content evenly between
		// the horcruxes
		writer = &multiplexing.Demultiplexer{Writers: horcruxFiles}
	} else {
		writers := make([]io.Writer, len(horcruxFiles))
		for i := range writers {
			writers[i] = horcruxFiles[i]
		}

		writer = io.MultiWriter(writers...)
	}

	_, err = io.Copy(writer, reader)
	if err != nil {
		return err
	}

	fmt.Println("Done!")

	return nil
}

func obtainTotalAndThreshold() (int, int, error) {
	totalPtr := flag.Int("n", 0, "number of horcruxes to make")
	thresholdPtr := flag.Int("t", 0, "number of horcruxes required to resurrect the original file")
	flag.Parse()

	total := *totalPtr
	threshold := *thresholdPtr

	if total == 0 {
		totalStr := prompt("How many horcruxes do you want to split this file into? (2-99): ")
		var err error
		total, err = strconv.Atoi(totalStr)
		if err != nil {
			return 0, 0, err
		}
	}

	if threshold == 0 {
		thresholdStr := prompt("How many horcruxes should be required to reconstitute the original file? If you require all horcruxes, the resulting files will take up less space, but it will feel less magical (2-99): ")
		var err error
		threshold, err = strconv.Atoi(thresholdStr)
		if err != nil {
			return 0, 0, err
		}
	}

	return total, threshold, nil
}

func header(index int, total int, headerBytes []byte) string {
	return fmt.Sprintf(`# THIS FILE IS A HORCRUX.
# IT IS ONE OF %d HORCRUXES THAT EACH CONTAIN PART OF AN ORIGINAL FILE.
# THIS IS HORCRUX NUMBER %d.
# IN ORDER TO RESURRECT THIS ORIGINAL FILE YOU MUST FIND THE OTHER %d HORCRUX(ES) AND THEN BIND THEM USING THE PROGRAM FOUND AT THE FOLLOWING URL
# https://github.com/jesseduffield/horcrux

-- HEADER --
%s
-- BODY --
`, total, index, total-1, headerBytes)
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}
