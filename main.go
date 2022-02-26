package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	// start := time.Now()

	if 1 >= len(os.Args) {
		fmt.Println("Usage: erase [path]")
		return
	}

	rootPath := os.Args[1]

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("You are about to permanently erase all files from: '", rootPath, "'\nAre you sure? This is IRREVERSIBLE (yes/no): ")
	ok := false
	for !ok {
		scanner.Scan()
		response := scanner.Text()
		if response == "no" {
			fmt.Println("Exiting...")
			return
		}

		if response != "yes" {
			fmt.Print("Write 'yes' or 'no': ")
			continue
		}
		ok = true
	}

	statusChan := Erase(rootPath)
	for status := range statusChan {
		fmt.Println(status)
	}
}
