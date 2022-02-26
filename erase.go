package main

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxFileDescriptors = 50

type statusType int

const (
	StatusTypeErr statusType = iota
	StatusTypeDiscovered
	StatusTypeDone
	StatusTypeFinal
)

type status struct {
	Message string
	Type    statusType
	Path    string

	AdditionalData interface{}
}

func (s status) String() string {
	var str string
	str += fmt.Sprintf("%-17s", s.Message+": ")
	switch s.Type {
	// In this case AdditionalData is of type error.
	case StatusTypeErr:
		if s.Path == "" {
			str += fmt.Sprintf("%s", s.AdditionalData)
		}
		str += fmt.Sprintf("%s: %s", s.AdditionalData, s.Path)

	default:
		str += fmt.Sprintf("'%s'", s.Path)
	}

	return str
}

func erase(rootPath string) <-chan status {
	statusChan := make(chan status)

	var absolutePathStatus *status
	absolutePath, err := filepath.Abs(rootPath)
	if err != nil {
		absolutePathStatus = &status{"Error while converting path to absolute", StatusTypeErr, "", err}
		return statusChan
	}

	friendlyStatusChan := make(chan status)
	go func() {
		unnecessaryPathPrefix := filepath.Dir(absolutePath) + string(filepath.Separator)

		for status := range statusChan {
			if status.Type != StatusTypeFinal {
				status.Path = strings.Replace(status.Path, unnecessaryPathPrefix, "", 1)
			}
			friendlyStatusChan <- status
		}
		close(friendlyStatusChan)
	}()

	go func() {
		if absolutePathStatus != nil {
			statusChan <- *absolutePathStatus
			return
		}

		filesToErasePathChan := make(chan string)

		var eraseWorkersWg sync.WaitGroup
		eraseWorkersWg.Add(maxFileDescriptors)

		for i := 0; i < maxFileDescriptors; i++ {
			go func() {
				for fileToErasePath := range filesToErasePathChan {
					eraseFile(fileToErasePath, statusChan)
				}
				eraseWorkersWg.Done()
			}()
		}

		err = filepath.WalkDir(absolutePath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				statusChan <- status{"Error while traversing", StatusTypeErr, path, err}
				return nil
			}

			dType := d.Type()
			if d.IsDir() || dType == fs.ModeSymlink || dType == fs.ModeDir {
				return nil
			}

			statusChan <- status{"Discovered file", StatusTypeDiscovered, path, nil}
			filesToErasePathChan <- path

			return nil
		})
		close(filesToErasePathChan)

		if err != nil {
			statusChan <- status{"Error while traversing", StatusTypeErr, absolutePath, err}
			return
		}

		eraseWorkersWg.Wait()
		statusChan <- status{"Erased requested path", StatusTypeFinal, absolutePath, nil}
		close(statusChan)
	}()

	return friendlyStatusChan
}

func eraseFile(fileToErasePath string, statusChan chan<- status) {
	file, err := os.OpenFile(fileToErasePath, os.O_WRONLY, 0)
	defer func() {
		err = file.Close()
		if err != nil {
			statusChan <- status{"Error while closing file", StatusTypeErr, fileToErasePath, err}
		}
	}()

	if err != nil {
		statusChan <- status{"Error while opening file", StatusTypeErr, fileToErasePath, err}
		return
	}

	fileInfo, err := file.Stat()
	if err != nil {
		statusChan <- status{"Error while reding file stats", StatusTypeErr, fileToErasePath, err}
		return
	}

	fileSize := fileInfo.Size()

	reportWriteErr := func(err error) {
		if err != nil {
			statusChan <- status{"Error while writing to file", StatusTypeErr, fileToErasePath, err}
		}
	}

	r := rand.New(cryptoSource{})
	reportWriteErr(writeBytes(file, fileSize, randomBytesGenerator(r.Int63())))
	reportWriteErr(writeBytes(file, fileSize, setBytesGenerator(0xFF)))
	reportWriteErr(writeBytes(file, fileSize, randomBytesGenerator(r.Int63())))
	reportWriteErr(writeBytes(file, fileSize, setBytesGenerator(0x00)))
	reportWriteErr(writeBytes(file, fileSize, setBytesGenerator(0xFF)))
	reportWriteErr(writeBytes(file, fileSize, randomBytesGenerator(r.Int63())))
	reportWriteErr(writeBytes(file, fileSize, setBytesGenerator(0x00)))

	err = file.Truncate(0)
	if err != nil {
		statusChan <- status{"Error while truncating file", StatusTypeErr, fileToErasePath, err}
	}

	statusChan <- status{"Erased file", StatusTypeDone, fileToErasePath, nil}
}

type byteGeneratorFunc func() (byte, error)

// BATCH_SIZE in byes, 5MB
const BATCH_SIZE = 5_000_000

func writeBytes(fd *os.File, size int64, byteGenerator byteGeneratorFunc) error {
	batchCount := size / BATCH_SIZE
	lastBatchSize := size - BATCH_SIZE*batchCount
	offset := int64(0)

	for i := 0; i < int(batchCount); i++ {
		batch := make([]byte, BATCH_SIZE)
		// Fill batch.
		for batchIndex := 0; batchIndex < BATCH_SIZE; batchIndex++ {
			b, err := byteGenerator()
			if err != nil {
				return err
			}

			batch[batchIndex] = b
		}

		_, err := fd.WriteAt(batch, offset)
		if err != nil {
			return err
		}

		offset += BATCH_SIZE
	}

	lastBatch := make([]byte, lastBatchSize)
	for batchIndex := 0; batchIndex < int(lastBatchSize); batchIndex++ {
		b, err := byteGenerator()
		if err != nil {
			return err
		}

		lastBatch[batchIndex] = b
	}

	_, err := fd.WriteAt(lastBatch, offset)
	if err != nil {
		return err
	}

	return nil
}

func randomBytesGenerator(seed int64) byteGeneratorFunc {
	r := rand.NewSource(seed)
	var bytes [8]byte
	offset := 0

	regenerate := func() {
		v := r.Int63()
		bytes = [8]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24), byte(v >> 32), byte(v >> 40), byte(v >> 48), byte(v >> 56)}
	}

	regenerate()

	return func() (byte, error) {
		if offset == 8 {
			regenerate()
			offset = 0
		}

		b := bytes[offset]
		offset++

		return b, nil
	}
}

func setBytesGenerator(setByte byte) byteGeneratorFunc {
	return func() (byte, error) {
		return setByte, nil
	}
}
