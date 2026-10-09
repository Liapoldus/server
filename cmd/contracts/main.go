// Command contracts exports Server's code-owned product declarations.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"liapoldus.local/server-plugin/contracts/definitions"
)

func main() {
	check := flag.Bool("check", false, "verify deterministic exports without writing")
	flag.Parse()
	if err := export(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func export(check bool) error {
	documents := definitions.Documents()
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)
	entries, err := os.ReadDir("contracts/v1")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("unexpected contract artifact: %s", entry.Name())
		}
		if _, ok := documents[entry.Name()]; !ok {
			return fmt.Errorf("unowned contract artifact: %s", entry.Name())
		}
	}
	for _, name := range names {
		// Normalize field ordering through the same JSON representation for every
		// declaration; struct layout must not affect artifact bytes.
		raw, err := json.Marshal(documents[name])
		if err != nil {
			return err
		}
		var value any
		if err = json.Unmarshal(raw, &value); err != nil {
			return err
		}
		contents, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		contents = append(contents, '\n')
		target := filepath.Join("contracts/v1", name)
		if check {
			existing, err := os.ReadFile(target)
			if err != nil {
				return err
			}
			if !bytes.Equal(existing, contents) {
				return fmt.Errorf("contract artifact out of date: %s", target)
			}
		} else if err = os.WriteFile(target, contents, 0o600); err != nil {
			return err
		}
	}
	return nil
}
