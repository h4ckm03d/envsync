package envsync

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

const (
	separator   = "="
	splitNumber = 2
	groupFmt    = "\n# %s\n"
	valueFmt    = "%s=%s\n"
)

// EnvSyncer describes some contracts to synchronize env.
type EnvSyncer interface {
	// Sync synchronizes source and target.
	// Source is the default env or the sample env.
	// Target is the actual env.
	// Both source and target are string and indicate the location of the files.
	//
	// Any values in source that aren't in target will be written to target.
	// Any values in source that are in target won't be written to target.
	Sync(source, target string) error
}

// Syncer implements EnvSyncer.
type Syncer struct {
}

// Sync implements EnvSyncer.
// Sync will read the file line by line.
// It will read the first '=' character.
// All characters prior to the first '=' character is considered as the key.
// All characters after the first '=' character until a newline character is considered as the value.
//
// e.g: FOO=bar.
// FOO is the key and bar is the value.
//
// During the synchronization process, there may be an error.
// Any key-values that have been synchronized before the error occurred is kept in target.
// Any key-values that haven't been synchronized because of an error occurred is ignored.
func (s *Syncer) Sync(source, target string) error {
	var err error
	backupFile := fmt.Sprintf("%s.bak", target)
	defer func(err error) {
		if err != nil {
			exec.Command("cp", backupFile, target).Run()
		}
		exec.Command("rm", "-f", backupFile).Run()
	}(err)

	// open the source file
	sFile, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "couldn't open source file")
	}
	defer sFile.Close()

	// open the target file
	tFile, err := os.OpenFile(target, os.O_APPEND|os.O_RDWR, os.ModeAppend)
	if err != nil {
		return errors.Wrap(err, "couldn't open target file")
	}
	defer tFile.Close()

	sMap, err := s.mapEnv(sFile)
	if err != nil {
		return err
	}

	tMap, err := s.mapEnv(tFile)
	if err != nil {
		return err
	}
	exec.Command("cp", "-f", target, backupFile).Run()
	newEnv, additionalEnv := s.appendNewEnv(sMap, tMap)
	s.print(additionalEnv)
	//clear current file
	tFile.Truncate(0)
	tFile.Seek(0, 0)
	err = s.writeEnv(tFile, newEnv)
	return errors.Wrap(err, "couldn't write target file")
}

func (s *Syncer) appendNewEnv(sMap, tMap map[string]string) (map[string]string, map[string]string) {
	addedEnv := make(map[string]string)
	for k, v := range sMap {
		if _, found := tMap[k]; !found {
			tMap[k] = v
			addedEnv[k] = v
		}
	}
	return tMap, addedEnv
}

func (s *Syncer) prefix(key string) string {
	return strings.Split(key, "_")[0]
}

func (s *Syncer) writeEnv(file *os.File, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort env before write
	group := ""
	groupComment := ""

	var buff bytes.Buffer

	for i, k := range keys {
		if g := s.prefix(k); g != group {
			if i == 0 {
				groupComment = "# %s\n"
			} else {
				groupComment = groupFmt
			}
			buff.WriteString(fmt.Sprintf(groupComment, g))
			group = g
		}
		buff.WriteString(fmt.Sprintf(valueFmt, k, env[k]))
	}

	if _, err := file.WriteString(buff.String()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error when writing file %s", buff.String()))
	}
	return nil
}

func (s *Syncer) print(env map[string]string) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort env before write
	if len(keys) > 0 {
		fmt.Println("New env added")
	}
	for _, k := range keys {
		fmt.Printf(valueFmt, k, env[k])
	}
}

func (s *Syncer) mapEnv(file *os.File) (map[string]string, error) {
	res := make(map[string]string)

	sc := bufio.NewScanner(file)
	sc.Split(bufio.ScanLines)

	for sc.Scan() {
		if sc.Text() != "" {
			if strings.HasPrefix(sc.Text(), "#") {
				continue
			}

			sp := strings.SplitN(sc.Text(), separator, splitNumber)
			if len(sp) != splitNumber {
				return res, fmt.Errorf("couldn't split %s by '=' into two strings", sc.Text())
			}

			res[sp[0]] = sp[1]
		}
	}

	return res, nil
}
