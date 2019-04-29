package main

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/schollz/progressbar/v2"
)

var cachefile = "/Users/jharnish/.dedupcache.json"
var files = make(map[[sha512.Size]byte]string)

type Cache struct {
	Files  map[string]string
	Hashes map[string][]string
}

type Dedupe struct {
	Unzip     bool
	Organize  bool
	Cache     Cache
	Verbose   bool
	Delete    bool
	Progress  bool
	FileCount int
	pb        *progressbar.ProgressBar
}

func NewCache(path string) *Dedupe {
	me := &Dedupe{}
	me.Unzip = false
	me.Organize = false
	me.Verbose = false
	me.Delete = false
	me.Progress = false
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
func (me *Dedupe) CreateProgressBar() {
	me.pb = progressbar.New(me.FileCount)
}

func (me *Dedupe) VerbosePrintln(output string) {
	if me.Verbose {
		fmt.Println(output)
	}
}

func (me *Dedupe) PurgeCache() {
	me.Cache = Cache{}
}

func (me *Dedupe) WriteCache(path string) {
	data, err := json.Marshal(me.Cache)
	if err != nil {
		fmt.Println(err)
	}
	ioutil.WriteFile(path, data, 0644)
}

func (me *Dedupe) checkDuplicate(path string, info os.FileInfo, err error) error {

	if err != nil {
		fmt.Println(err)
		return nil
	}
	if info.IsDir() { // skip directory
		return nil
	}
	me.VerbosePrintln(path)
	if me.Progress {
		me.pb.Add(1)
	}
	if strings.HasSuffix(info.Name(), ".zip") {
		me.VerbosePrintln("Unzipping " + path)
		_, err := Unzip(path, "")
		if err != nil {
			log.Fatal(err)
		} else {
			os.Remove(path)
		}
		return nil

		//unzip and delete
		// add new files to the list?
	}
	if _, ok := me.Cache.Files[path]; ok {
		me.VerbosePrintln("Skipping " + path + " already in cache")
		//fmt.Println("Already looked at", path, val)
		return nil
	}
	if me.Organize {

		pathparts := strings.Split(path, "/")
		filename := pathparts[len(pathparts)-1]
		newdir := strings.ToLower(filename[0:2])
		if !DirExists(newdir) {
			os.Mkdir(newdir, 0755)
		}
		if path != newdir+"/"+filename {
			me.VerbosePrintln("Organizing " + path)
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

	me.Cache.Hashes[hashstr] = AppendIfMissing(me.Cache.Hashes[hashstr], path) // store in map for comparison

	if me.Cache.Files == nil {
		me.Cache.Files = make(map[string]string)
	}
	me.Cache.Files[path] = hashstr

	return nil
}
func (me *Dedupe) ShowDuplicates() {
	for val := range me.Cache.Hashes {
		if len(me.Cache.Hashes[val]) > 1 {
			fmt.Println("Duplicates ", me.Cache.Hashes[val])
		}

	}
}
func (me *Dedupe) DeleteDuplicates() {
	for val := range me.Cache.Hashes {
		if len(me.Cache.Hashes[val]) > 1 {
			fmt.Println("Duplicates ", me.Cache.Hashes[val])
		}

	}
}

func main() {

	if len(os.Args) < 2 {
		fmt.Printf("USAGE : %s [options] <target_directory> \n", os.Args[0])
		fmt.Println("-c purges the cache")
		fmt.Println("-s sorts the files into subdirectories")
		fmt.Println("-z unzip any zipfiles")
		fmt.Println("-d deletes duplicates")
		fmt.Println("-v verbose output, show each file looked at")
		fmt.Println("-p progress bar")
		os.Exit(0)
	}
	c := NewCache(cachefile)
	dir := ""
	for idx := range os.Args {
		switch os.Args[idx] {
		case "-c":
			c.PurgeCache()
		case "-s":
			c.Organize = true
		case "-z":
			c.Unzip = true
		case "-v":
			c.Verbose = true
		case "-d":
			c.Delete = true
		case "-p":
			c.CountFiles()
			c.Progress = true
			c.CreateProgressBar()
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

func Unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func AppendIfMissing(slice []string, i string) []string {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

func (me *Dedupe) CountFiles() {
	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				me.FileCount++
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}
	fmt.Println("File Count", me.FileCount)
}
