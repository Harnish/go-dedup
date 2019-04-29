package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

var cachefile = "/Users/jharnish/.dedupcache.json"
var files = make(map[[sha512.Size]byte]string)

type Cache struct {
	Files  map[string]string
	Hashes map[string][]string
}

type Dedupe struct {
	Unzip    bool
	Organize bool
	Cache    Cache
}

func NewCache(path string) *Dedupe {
	me := &Dedupe{}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		return me
	}

	err = json.Unmarshal(data, me)
	if err != nil {
		fmt.Println(err)

		return me
	}
	return me
}

func PurgeCache(path string) {
	me := &Dedupe{}
	me.WriteCache(path)
}

func (me *Dedupe) WriteCache(path string) {
	data, err := json.Marshal(me.Cache)
	if err != nil {
		fmt.Println(err)
	}
	ioutil.WriteFile(path, data, 0644)
}

func (me *Dedupe) checkDuplicate(path string, info os.FileInfo, err error) error {
	//fmt.Println(path)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	if info.IsDir() { // skip directory
		return nil
	}
	if strings.HasSuffix(info.Name(), ".zip") {
		//unzip and delete
		// add new files to the list?
	}
	if _, ok := me.Cache.Files[path]; ok {
		//fmt.Println("Already looked at", path, val)
		return nil
	}
	if me.Organize {
		pathparts := strings.Split(path, "/")
		filename := pathparts[len(pathparts)-1]
		newdir := filename[0:2]
		if !DirExists(newdir) {
			os.Mkdir(newdir, 0755)
		}
		if path != newdir+"/"+filename {
			os.Rename(path, newdir+"/"+filename)
			path = newdir + "/" + filename
		}

	}
	data, err := ioutil.ReadFile(path)

	if err != nil {
		fmt.Println(err)
		return nil
	}
	hash := sha256.Sum256(data) // get the file sha512 hash
	hashstr := hex.EncodeToString(hash[:])

	if me.Cache.Hashes == nil {
		me.Cache.Hashes = make(map[string][]string)
	}
	me.Cache.Hashes[hashstr] = append(me.Cache.Hashes[hashstr], path) // store in map for comparison
	if me.Cache.Files == nil {
		me.Cache.Files = make(map[string]string)
	}
	me.Cache.Files[path] = hashstr

	return nil
}
func (me *Dedupe) ShowDuplicates() {
	for val := range me.Cache.Hashes {
		if len(me.Cache.Hashes[val]) > 0 {
			fmt.Println("Duplicates ", me.Cache.Hashes[val])
		}

	}
}
func (me *Cache) DeleteDuplicates() {

}

func main() {
	organize := false
	if len(os.Args) < 2 {
		fmt.Printf("USAGE : %s [-c -s -z] <target_directory> \n", os.Args[0])
		fmt.Println("-c purges the cache")
		fmt.Println("-s sorts the files into subdirectories")
		fmt.Println("-z unzip any zipfiles")
		os.Exit(0)
	}
	dir := ""
	for idx := range os.Args {
		switch os.Args[idx] {
		case "-c":
			PurgeCache(cachefile)
		case "-s":
			organize = true
		default:
			dir = os.Args[idx]
		}
	}
	if dir == "" {
		fmt.Printf("USAGE : %s [-c -s -z] <target_directory> \n", os.Args[0])
		fmt.Println("-c purges the cache")
		fmt.Println("-s sorts the files into subdirectories")
		fmt.Println("-z unzip any zipfiles")
		os.Exit(0)
	}
	c := NewCache(cachefile)
	c.Organize = organize
	cx := make(chan os.Signal)
	signal.Notify(cx, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-cx
		c.WriteCache(cachefile)
		os.Exit(1)
	}()
	err := filepath.Walk(dir, c.checkDuplicate)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	c.ShowDuplicates()
	c.WriteCache(cachefile)
}

func DirExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	} else {
		return true
	}
	return false
}
