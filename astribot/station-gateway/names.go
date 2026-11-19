package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// namesMarker ends every line of the node's hosts file that this program
// owns. Nothing else writes lines with it.
const namesMarker = " # rlark names"

// writeNames makes the marked lines of the hosts file the given names, none
// when there are none, and leaves every other line alone. The file keeps its
// place: it is opened by its path every time, so a file that something else
// put there under that name is followed, and it is written over from its
// start and cut to the new length afterwards, so that no failure in between
// leaves it empty. Host-network pods get a copy of the file when their
// containers start; what must follow a changed address is started again by
// whoever changes it.
func writeNames(path string, names []hostName) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(current), "\n"), "\n") {
		if !strings.HasSuffix(line, namesMarker) {
			lines = append(lines, line)
		}
	}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s %s%s", name.Address, name.Name, namesMarker))
	}
	wanted := strings.Join(lines, "\n") + "\n"
	if wanted == string(current) {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteAt([]byte(wanted), 0); err != nil {
		return err
	}
	return file.Truncate(int64(len(wanted)))
}

// writeResolv keeps the resolv.conf the node's kubelet gives to pods: the
// site's name servers where its network declares them, else what the machine
// itself uses, the first of its files that exists. Declaring name servers,
// or taking the declaration back, so reaches a node without anyone going to it.
func writeResolv(path string, servers []string, machine []string) error {
	wanted := ""
	for _, server := range servers {
		wanted += "nameserver " + server + "\n"
	}
	if len(servers) == 0 {
		for _, file := range machine {
			if own, err := os.ReadFile(file); err == nil {
				wanted = string(own)
				break
			}
		}
	}
	if current, err := os.ReadFile(path); err == nil && string(current) == wanted {
		return nil
	}
	// A node the installer did not make has no directory for it yet.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return replaceFile(path, strings.NewReader(wanted), 0644)
}
