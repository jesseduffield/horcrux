package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
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

type horcruxHeader struct {
	OriginalFilename string `json:"originalFilename"`
	Timestamp        int64  `json:"timestamp"`
	Index            int    `json:"index"`
	Total            int    `json:"total"`
	Threshold        int    `json:"threshold"`
	KeyFragment      []byte `json:"keyFragment"`
}

func main() {
	// I'd use `flaggy` but I like the idea of this repo having no dependencies
	// Unfortunately that means I'm awkwardly making use of the standard flag package
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[len(os.Args)-2] {

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
		path := os.Args[len(os.Args)-1]
		if err := split(path); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

func usage() {
	log.Fatal("usage: `horcrux bind [<directory>]` | `horcrux [-t] [-n] split <filename>`\n-n: number of horcruxes to make\n-t: number of horcruxes required to resurrect the original file\nexample: horcrux -t 3 -n 5 split diary.txt")
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func split(path string) error {
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

func bind(dir string) error {
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

		if len(headers) > 0 && currentHeader.OriginalFilename != headers[0].OriginalFilename || currentHeader.Timestamp != headers[0].Timestamp {
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
