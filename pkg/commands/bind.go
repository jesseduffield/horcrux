package commands

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jesseduffield/horcrux/pkg/multiplexing"
	"github.com/jesseduffield/horcrux/pkg/shamir"
)

func GetHorcruxPathsInDir(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	paths := []string{}
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".horcrux" {
			path := filepath.Join(dir, file.Name())
			paths = append(paths, path)
		}
	}

	return paths, nil
}

type byIndex []Horcrux

func (h byIndex) Len() int {
	return len(h)
}

func (h byIndex) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h byIndex) Less(i, j int) bool {
	return h[i].GetHeader().Index < h[j].GetHeader().Index
}

func GetHorcruxes(paths []string) ([]Horcrux, error) {
	horcruxes := []Horcrux{}

	for _, path := range paths {
		currentHorcrux, err := NewHorcrux(path)
		if err != nil {
			return nil, err
		}
		for _, horcrux := range horcruxes {
			if horcrux.GetHeader().Index == currentHorcrux.GetHeader().Index && horcrux.GetHeader().OriginalFilename == currentHorcrux.GetHeader().OriginalFilename {
				// we've already obtained this horcrux so we'll skip this instance
				continue
			}
		}

		horcruxes = append(horcruxes, *currentHorcrux)
	}

	sort.Sort(byIndex(horcruxes))

	return horcruxes, nil
}

func ValidateHorcruxes(horcruxes []Horcrux) error {
	if len(horcruxes) == 0 {
		return errors.New("No horcruxes supplied")
	}

	if len(horcruxes) < horcruxes[0].GetHeader().Threshold {
		return fmt.Errorf(
			"You do not have all the required horcruxes. There are %d required to resurrect the original file. You only have %d",
			horcruxes[0].GetHeader().Threshold,
			len(horcruxes),
		)
	}

	for _, horcrux := range horcruxes {
		if !strings.HasSuffix(horcrux.GetPath(), ".horcrux") {
			return fmt.Errorf("%s is not a horcrux file (requires .horcrux extension)", horcrux.GetPath())
		}
		if horcrux.GetHeader().OriginalFilename != horcruxes[0].GetHeader().OriginalFilename || horcrux.GetHeader().Timestamp != horcruxes[0].GetHeader().Timestamp {
			return errors.New("All horcruxes in the given directory must have the same original filename and timestamp.")
		}
	}

	return nil
}

func Bind(paths []string, dstPath string, overwrite bool) error {
	horcruxes, err := GetHorcruxes(paths)
	if err != nil {
		return err
	}

	if err := ValidateHorcruxes(horcruxes); err != nil {
		return err
	}

	firstHorcrux := horcruxes[0]

	// if dstPath is empty we use the original filename
	if dstPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dstPath = filepath.Join(cwd, firstHorcrux.GetHeader().OriginalFilename)
	}

	if fileExists(dstPath) && !overwrite {
		return os.ErrExist
	}

	keyFragments := make([][]byte, len(horcruxes))
	for i := range keyFragments {
		keyFragments[i] = horcruxes[i].GetHeader().KeyFragment
	}

	key, err := shamir.Combine(keyFragments)
	if err != nil {
		return err
	}

	var fileReader io.Reader
	if firstHorcrux.GetHeader().Total == firstHorcrux.GetHeader().Threshold {
		horcruxFiles := make([]*os.File, len(horcruxes))
		for i, horcrux := range horcruxes {
			horcruxFiles[i] = horcrux.GetFile()
		}

		fileReader = &multiplexing.Multiplexer{Readers: horcruxFiles}
	} else {
		fileReader = firstHorcrux.GetFile() // arbitrarily read from the first horcrux: they all contain the same contents
	}

	reader := cryptoReader(fileReader, key)

	_ = os.Truncate(dstPath, 0)

	newFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE, 0644)
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
