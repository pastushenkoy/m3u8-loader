package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/grafov/m3u8"
	"gopkg.in/cheggaaa/pb.v1"
)

const workersCount = 10

func main() {
	defaultStr := ""

	flag.Parse()

	url := flag.Arg(0)
	out := flag.Arg(1)

	if url == defaultStr || out == defaultStr {
		fmt.Println("Error: url and output file must be specified")
		return
	}

	mediapl, err := GetMediaPlaylist(url)
	if err != nil {
		fmt.Println(err)
	}

	dir, err := ioutil.TempDir("./", "*")
	if err != nil {
		fmt.Println("Error creating temp dir")
		return
	}
	defer os.RemoveAll(dir)

	err = DownloadSegments(mediapl, dir)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = MergeSegments(dir, out)
	if err != nil {
		fmt.Printf("Error merging segments: %s", err)
		return
	}
}

func MergeSegments(dir string, out string) error {
	outUpper := fmt.Sprintf("../%s", out)
	cmd := exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", "segments.txt", "-y", "-c", "copy", outUpper)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	fmt.Println(cmd)

	return cmd.Run()
}

func GetMediaPlaylist(url string) (*m3u8.MediaPlaylist, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%d error downloading playlist: '%s'", resp.StatusCode, url)
	}

	p, listType, err := m3u8.DecodeFrom(resp.Body, false)
	if err != nil {
		return nil, err
	}

	switch listType {
	case m3u8.MEDIA:
		mediapl := p.(*m3u8.MediaPlaylist)
		return mediapl, nil
	case m3u8.MASTER:
		masterpl := p.(*m3u8.MasterPlaylist)
		return PickMediaFromMaster(masterpl)
	default:
		return nil, fmt.Errorf("unknown playlist format: %+v", listType)
	}
}

func PickMediaFromMaster(masterpl *m3u8.MasterPlaylist) (*m3u8.MediaPlaylist, error) {
	fmt.Println("This is a master playlist:")
	for i, v := range masterpl.Variants {
		fmt.Printf("[%d] %s %s\n", i+1, v.Name, v.Resolution)
	}
	fmt.Printf("Pick a number, you want to download: ")
	for {
		var input string
		fmt.Scanln(&input)
		number, err := strconv.Atoi(input)
		if err == nil {
			if number >= 1 && number <= len(masterpl.Variants) {
				return GetMediaPlaylist(masterpl.Variants[number-1].URI)
			}
		}
		fmt.Printf("Enter number from 1 to %d\n", len(masterpl.Variants))
	}
}

type downloadTask struct {
	source      string
	destination string
}

func DownloadSegments(mediapl *m3u8.MediaPlaylist, dir string) error {
	fmt.Printf("Found %d parts. Downloading...\n", mediapl.Count())

	segmentsFile, err := os.Create(fmt.Sprintf("%s/segments.txt", dir))
	if err != nil {
		return err
	}
	defer segmentsFile.Close()

	bar := pb.StartNew((int)(mediapl.Count()))

	jobs := make(chan downloadTask, workersCount)
	errors := make(chan error)
	done := make(chan int, workersCount)

	for i := 0; i < workersCount; i++ {
		go func() {
			for w := range jobs {
				err := DownloadSegment(w.source, w.destination)
				if err != nil {
					errors <- err
				}
				bar.Increment()
			}

			done <- 1
		}()
	}

	go func() {
		for i := uint(0); i < mediapl.Count(); i++ {
			seg := mediapl.Segments[i]
			fileName := fmt.Sprintf("%04d.ts", i)
			pathToFile := filepath.Join(".", dir, fileName)

			io.WriteString(segmentsFile, fmt.Sprintf("file '%s'\n", fileName))

			jobs <- downloadTask{source: seg.URI, destination: pathToFile}
		}
		close(jobs)
	}()

	finishedWorkers := 0
	for {
		select {
		case err = <-errors:
			return err
		case <-done:
			finishedWorkers++
			if finishedWorkers == workersCount {
				bar.Finish()
				return nil
			}
		}
	}
}

func DownloadSegment(url string, p string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
