package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jesseduffield/horcrux/pkg/multiplexing"
	"github.com/jesseduffield/horcrux/pkg/shamir"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "bind":
		var dir string
		if len(os.Args) == 2 {
			dir = "."
		} else {
			dir = os.Args[2]
		}
		if err := bind(dir); err != nil {
			log.Fatal(err)
		}
	case "split":
		if len(os.Args) == 2 {
			usage()
		}
		path := os.Args[2]
		if err := split(path); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

func usage() {
	log.Fatal("usage: `horcrux bind [<directory>]` | `horcrux split <filename>`")
}

type horcruxHeader struct {
	OriginalFilename string `json:"originalFilename"`
	Timestamp        int64  `json:"timestamp"`
	Index            int    `json:"index"`
	Total            int    `json:"total"`
	Threshold        int    `json:"threshold"`
	KeyFragment      []byte `json:"keyFragment"`
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func split(path string) error {
	totalStr := prompt("How many horcruxes do you want to split this file into? (1-99): ")
	total, err := strconv.Atoi(totalStr)
	if err != nil {
		return err
	}

	thresholdStr := prompt("How many horcruxes should be required to reconstitute the original file? If you require all horcruxes, the resulting files will take up less space, but it will feel less magical (1-99): ")
	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil {
		return err
	}

	key, err := generateKey()
	if err != nil {
		return err
	}

	byteArrs, err := shamir.Split(key, total, threshold)
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

	var r io.Reader = file

	for i := range horcruxFiles {
		index := i + 1

		h := horcruxHeader{
			OriginalFilename: originalFilename,
			Timestamp:        timestamp,
			Index:            index,
			Total:            total,
			KeyFragment:      byteArrs[i],
			Threshold:        threshold,
		}

		bytes, err := json.Marshal(&h)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		originalFilenameWithoutExt := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
		horcruxFilename := fmt.Sprintf("%s_%d_of_%d.horcrux", originalFilenameWithoutExt, index, total)
		fmt.Printf("creating %s\n", horcruxFilename)
		_ = os.Truncate(horcruxFilename, 0)
		horcruxFiles[i], err = os.OpenFile(horcruxFilename, os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			return err
		}

		horcruxFile := horcruxFiles[i]
		defer horcruxFile.Close()
		horcruxFile.WriteString(header(index, total))
		horcruxFile.Write(bytes)
		horcruxFile.WriteString("\n-- BODY --\n")
	}

	r = cryptoReader(r, key)

	writers := make([]io.Writer, len(horcruxFiles))
	for i := range writers {
		writers[i] = horcruxFiles[i]
	}

	var w io.Writer
	if threshold == total {
		w = &multiplexing.Demultiplexer{Writers: horcruxFiles} // TODO: use the writers here too
	} else {
		w = io.MultiWriter(writers...)
	}

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}

	fmt.Println("Done!")

	return nil
}

func header(index int, total int) string {
	return fmt.Sprintf(`# THIS FILE IS A HORCRUX.
# IT IS ONE OF %d HORCRUXES THAT EACH CONTAIN PART OF AN ORIGINAL FILE.
# THIS IS HORCRUX NUMBER %d.
# IN ORDER TO RESURRECT THIS ORIGINAL FILE YOU MUST FIND THE OTHER %d HORCRUX(ES) AND THEN BIND THEM USING THE PROGRAM FOUND AT THE FOLLOWING URL
# https://github.com/jesseduffield/horcrux

-- HEADER --
`, total, index, total-1)
}

func bind(dir string) error {
	// get all the horcrux files within the directory
	filenames := []string{}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".horcrux" {
			filenames = append(filenames, file.Name())
		}
	}

	var originalFilename string
	var timestamp int64
	var total int
	var threshold int

	horcruxes := []horcruxHeader{}
	horcruxFiles := []*os.File{}

	for _, filename := range filenames {
		file, err := os.Open(filename)
		defer file.Close()
		if err != nil {
			return err
		}

		scanner := bufio.NewScanner(file)
		header := &horcruxHeader{}
		bytesBeforeBody := 0
		for scanner.Scan() {
			line := scanner.Text()
			bytesBeforeBody += len(scanner.Bytes()) + 1
			if line == "-- HEADER --" {
				scanner.Scan()
				bytesBeforeBody += len(scanner.Bytes()) + 1
				headerLine := scanner.Bytes()
				json.Unmarshal(headerLine, header)
				scanner.Scan() // one more to get past the body line (TODO: just remove the body line)
				bytesBeforeBody += len(scanner.Bytes()) + 1
				break
			}
		}
		if _, err := file.Seek(int64(bytesBeforeBody), io.SeekStart); err != nil {
			return err
		}

		if header == nil {
			return errors.New("could not find header in horcrux file")
		}

		for _, horcrux := range horcruxes {
			if horcrux.Index == header.Index {
				// we've already obtained this horcrux so we'll skip this one
				continue
			}
		}

		if originalFilename == "" {
			originalFilename = header.OriginalFilename
			timestamp = header.Timestamp
			total = header.Total
			threshold = header.Threshold
		} else {
			if header.OriginalFilename != originalFilename || header.Timestamp != timestamp {
				return errors.New("All horcruxes in the given directory must have the same original filename and timestamp.")
			}
		}

		horcruxes = append(horcruxes, *header)
		horcruxFiles = append(horcruxFiles, file)
	}

	if total == 0 {
		return errors.New("No horcruxes in directory")
	}

	// check that we have the threshold.
	if len(horcruxes) < threshold {
		return errors.New(fmt.Sprintf("You do not have all the required horcruxes. There are %d required to resurrect the original file. You only have %d", threshold, len(horcruxes)))
	}

	keyFragments := make([][]byte, len(horcruxes))
	for i := range keyFragments {
		keyFragments[i] = horcruxes[i].KeyFragment
	}

	key, err := shamir.Combine(keyFragments)
	if err != nil {
		return err
	}

	var r io.Reader
	if total == threshold {
		// sort by index
		orderedHorcruxes := make([]horcruxHeader, len(horcruxes))
		for _, h := range horcruxes {
			orderedHorcruxes[h.Index-1] = h
		}

		r = &multiplexing.Multiplexer{Readers: horcruxFiles}
	} else {
		r = horcruxFiles[0] // arbitrarily read from the first horcrux: they all contain the same contents
	}

	r = cryptoReader(r, key)

	newFilename := originalFilename
	if fileExists(originalFilename) {
		newFilename = prompt("A file already exists named '%s'. Enter new file name: ", originalFilename)
	}

	_ = os.Truncate(newFilename, 0)

	newFile, err := os.OpenFile(newFilename, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, r)
	if err != nil {
		return err
	}

	return err
}

func cryptoReader(r io.Reader, key []byte) io.Reader {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(block, iv[:])

	return cipher.StreamReader{S: stream, R: r}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func prompt(message string, args ...interface{}) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf(message, args...)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
