package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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

	"gopkg.in/yaml.v2"
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

type horcrux struct {
	OriginalFilename string `yaml:"originalFilename"`
	Timestamp        int64  `yaml:"timestamp"`
	Index            int    `yaml:"index"`
	Total            int    `yaml:"total"`
	Key              string `yaml:"key"`
	EncryptedContent string `yaml:"encryptedContent"`
}

func split(path string) error {
	totalStr := prompt("How many horcruxes do you want to split this file into? (0-99): ")
	total, err := strconv.Atoi(totalStr)
	if err != nil {
		return err
	}

	timestamp := time.Now().Unix()

	contentBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	// I need <total> keys and I need to encrypt this thing with each key in sequence
	keys := make([][]byte, total)

	for i := range keys {
		keys[i] = make([]byte, 32)
		_, err = rand.Read(keys[i])
		if err != nil {
			return err
		}

		contentBytes = encrypt(contentBytes, keys[i])
	}

	base64EncodedContent := base64.StdEncoding.EncodeToString(contentBytes)

	splitContent := splitIntoEqualParts(base64EncodedContent, total)

	originalFilename := filepath.Base(path)

	for i := range keys {
		strKey := base64.StdEncoding.EncodeToString(keys[i])
		index := i + 1

		h := horcrux{
			OriginalFilename: originalFilename,
			Timestamp:        timestamp,
			Index:            index,
			Total:            total,
			Key:              strKey,
			EncryptedContent: splitContent[i],
		}

		bytes, err := yaml.Marshal(&h)
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		fileContent := append([]byte(header(index, total)), bytes...)

		originalFilenameWithoutExt := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
		horcruxFilename := fmt.Sprintf("%s_%d_of_%d.horcrux", originalFilenameWithoutExt, index, total)
		fmt.Printf("creating %s\n", horcruxFilename)
		ioutil.WriteFile(horcruxFilename, fileContent, 0644)
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

`, total, index, total-1)
}

func bind(dir string) error {
	// get all the horcrux files within the directory
	filenames := []string{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".horcrux" {
			filenames = append(filenames, path)
		}
		return nil
	})

	var originalFilename string
	var timestamp int64
	var total int
	var indices []int

	horcruxes := []horcrux{}

	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			return err
		}

		reader := bufio.NewReader(file)

		decoder := yaml.NewDecoder(reader)
		var h horcrux
		decoder.Decode(&h)

		for _, index := range indices {
			if index == h.Index {
				// we've already obtained this horcrux so we'll skip this one
				continue
			}
		}
		indices = append(indices, h.Index)

		if originalFilename == "" {
			originalFilename = h.OriginalFilename
			timestamp = h.Timestamp
			total = h.Total
		} else {
			if h.OriginalFilename != originalFilename || h.Timestamp != timestamp {
				return errors.New("All horcruxes in the given directory must have the same original filename and timestamp.")
			}
		}

		horcruxes = append(horcruxes, h)
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
	orderedHorcruxes := make([]horcrux, len(horcruxes))
	for _, h := range horcruxes {
		orderedHorcruxes[h.Index-1] = h
	}

	// now we just need to concatenate the contents together, decode the base64 encoding, then decrypt everything with the first to the last key
	encodedContent := ""
	for _, h := range orderedHorcruxes {
		encodedContent += h.EncryptedContent
	}

	decodedContentBytes, err := base64.StdEncoding.DecodeString(encodedContent)
	if err != nil {
		return err
	}

	// decrypt in reverse order to how we encrypted
	for i := range orderedHorcruxes {
		bytesKey, err := base64.StdEncoding.DecodeString(orderedHorcruxes[total-i-1].Key)
		if err != nil {
			return err
		}
		decodedContentBytes = decrypt(decodedContentBytes, bytesKey)
	}

	newFilename := originalFilename
	if fileExists(originalFilename) {
		newFilename = prompt("A file already exists named '%s'. Enter new file name: ", originalFilename)
	}

	if err := ioutil.WriteFile(newFilename, decodedContentBytes, 0644); err != nil {
		return err
	}

	return err
}

// see https://www.thepolyglotdeveloper.com/2018/02/encrypt-decrypt-data-golang-application-crypto-packages/
func encrypt(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext
}

// see https://www.thepolyglotdeveloper.com/2018/02/encrypt-decrypt-data-golang-application-crypto-packages/
func decrypt(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		panic(err.Error())
	}
	return plaintext
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

func splitIntoEqualParts(s string, n int) []string {
	runes := bytes.Runes([]byte(s))
	sliceLength := len(runes) / n
	slices := make([]string, n)
	for i := range slices {
		if i == n-1 {
			slices[i] = string(runes[i*sliceLength:])
		} else {
			slices[i] = string(runes[i*sliceLength : (i+1)*sliceLength])
		}
	}

	return slices
}
