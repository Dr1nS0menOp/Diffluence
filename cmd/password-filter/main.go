// SPDX-License-Identifier: Apache-2.0

// Command password-filter builds the Bloom filter of leaked passwords from a list
// with one password per line, such as the SecLists file named in
// docs/dependencies.md:
//
//	go run ./cmd/password-filter list.txt internal/identity/leaked/passwords.bloom
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/diffluence/diffluence/internal/identity/leaked"
)

// At 16 bits per entry and 11 hashes, about 1 in 2000 other passwords is refused
// by mistake, which only means choosing another one.
const (
	bitsPerEntry = 16
	hashes       = 11
)

func readList(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- a path given by the person running this tool
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	seen := map[string]bool{}
	var out []string
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1<<20), 1<<20)
	for scan.Scan() {
		line := strings.ToLower(strings.TrimRight(scan.Text(), "\r"))
		if line != "" && !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	return out, scan.Err()
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: password-filter <list> <output>")
	}
	list, err := readList(args[0])
	if err != nil {
		return err
	}
	filter := leaked.New(uint64(len(list)), bitsPerEntry, hashes)
	for _, password := range list {
		filter.Add(password)
	}
	if err := os.WriteFile(args[1], filter.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Printf("%d passwords\n", len(list))
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
