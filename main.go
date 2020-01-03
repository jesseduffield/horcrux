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
)

const BYTE_QUOTA = 100

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
	KeyFragment      []byte `json:"keyFragment"`
}

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func split(path string) error {
	totalStr := prompt("How many horcruxes do you want to split this file into? (0-99): ")
	total, err := strconv.Atoi(totalStr)
	if err != nil {
		return err
	}

	timestamp := time.Now().Unix()

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	originalFilename := filepath.Base(path)

	key, err := generateKey()
	if err != nil {
		return err
	}
	splitKey := splitIntoEqualPartsBytes(key, total)

	horcruxFiles := make([]*os.File, total)

	for i := range horcruxFiles {
		index := i + 1

		h := horcruxHeader{
			OriginalFilename: originalFilename,
			Timestamp:        timestamp,
			Index:            index,
			Total:            total,
			KeyFragment:      splitKey[i],
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

	w := &demultiplexer{writers: horcruxFiles}
	r := encrypt(file, key)

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}

	fmt.Println("Done!")

	return nil
}

func min(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

type demultiplexer struct {
	writers      []*os.File
	writerIndex  int
	bytesWritten int
}

func (d *demultiplexer) nextWriter() {
	d.writerIndex++
	if d.writerIndex > len(d.writers)-1 {
		d.writerIndex = 0
	}
	d.bytesWritten = 0
}

func (d *demultiplexer) Write(p []byte) (int, error) {
	totalN := 0
	for totalN < len(p) {
		remainingBytes := len(p) - totalN
		remainingBytesForWriter := BYTE_QUOTA - d.bytesWritten
		n, err := d.writers[d.writerIndex].Write(p[totalN : totalN+min(remainingBytesForWriter, remainingBytes)])
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

type multiplexer struct {
	readers     []*os.File
	readerIndex int
	bytesRead   int
}

func (m *multiplexer) nextReader() {
	m.readerIndex++
	if m.readerIndex > len(m.readers)-1 {
		m.readerIndex = 0
	}
	m.bytesRead = 0
}

func (m *multiplexer) Read(p []byte) (int, error) {
	totalN := 0
	for totalN < len(p) {
		remainingBytes := len(p) - totalN
		remainingBytesForReader := BYTE_QUOTA - m.bytesRead
		buf := make([]byte, min(remainingBytes, remainingBytesForReader))
		n, err := m.readers[m.readerIndex].Read(buf)
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

	// check that we have the total.
	if len(horcruxes) < total {
		horcruxIndices := make([]string, len(horcruxes))
		for i, h := range horcruxes {
			horcruxIndices[i] = strconv.Itoa(h.Index)
		}

		return errors.New(fmt.Sprintf("You do not have all the required horcruxes. There are %d in total. You only have horcrux(es) %s", total, strings.Join(horcruxIndices, ",")))
	}

	// sort by index
	orderedHorcruxes := make([]horcruxHeader, len(horcruxes))
	for _, h := range horcruxes {
		orderedHorcruxes[h.Index-1] = h
	}

	// now we just need to concatenate the contents together then decrypt everything with the first to the last key
	var key []byte
	for _, h := range orderedHorcruxes {
		key = append(key, h.KeyFragment...)
	}

	r := &multiplexer{readers: horcruxFiles}

	decryptReader := decrypt(r, key)

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

	_, err = io.Copy(newFile, decryptReader)
	if err != nil {
		return err
	}

	return err
}

func encrypt(r io.Reader, key []byte) io.Reader {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(block, iv[:])

	return cipher.StreamReader{S: stream, R: r}
}

// see https://www.thepolyglotdeveloper.com/2018/02/encrypt-decrypt-data-golang-application-crypto-packages/
func decrypt(r io.Reader, key []byte) io.Reader {
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

func splitIntoEqualPartsBytes(s []byte, n int) [][]byte {
	sliceLength := len(s) / n
	slices := make([][]byte, n)
	for i := range slices {
		if i == n-1 {
			slices[i] = s[i*sliceLength:]
		} else {
			slices[i] = s[i*sliceLength : (i+1)*sliceLength]
		}
	}

	return slices
}
