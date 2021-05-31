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

	files, err := DownloadSegments(mediapl, dir)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = MergeSegments(dir, files, out)
	if err != nil {
		fmt.Printf("Error merging segments: %s", err)
		return
	}
}

func MergeSegments(dir string, files []string, out string) error {
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

func DownloadSegments(mediapl *m3u8.MediaPlaylist, dir string) ([]string, error) {
	fmt.Printf("Found %d parts. Downloading...\n", mediapl.Count())

	createdFiles := make([]string, 0)

	f, err := os.Create(fmt.Sprintf("%s/segments.txt", dir))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bar := pb.StartNew((int)(mediapl.Count()))

	for i := uint(0); i < mediapl.Count(); i++ {
		seg := mediapl.Segments[i]
		// fmt.Printf("%d/%d Starting part\n", i+1, mediapl.Count())
		fn := fmt.Sprintf("%04d.ts", i)
		fileName := filepath.Join(".", dir, fn)
		err := DownloadSegment(seg.URI, fileName)
		if err != nil {
			return nil, err
		}

		io.WriteString(f, fmt.Sprintf("file '%s'\n", fn))

		bar.Increment()
		createdFiles = append(createdFiles, fileName)
		// fmt.Printf("%d/%d Finished part\r\n", i+1, mediapl.Count())
	}

	bar.Finish()

	return createdFiles, nil
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
