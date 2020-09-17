package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	CanNotGetMediaInfo    = 101
	YoutubeDLCommandError = 102
	ConvertSuccess        = 103
)

type VideoInfo struct {
	UploadDate    string `json:"upload_date"`
	VideoDuration int    `json:"duration"`
	Title         string `json:"title"`
	Ext           string `json:"ext"`
	Uploader      string `json:"uploader"`
	Description   string `json:"description"`
	Extractor     string `json:"extractor"`
}

type MediaInfo struct {
	VideoInfo      VideoInfo `json:"video_info"`
	DownloadUrl    string    `json:"download_url"`
	ErrCode        int       `json:"error_code"`
	ProcessPercent string    `json:"process_percentage"`
	FileSize       []int64   `json:"file_size"`
}

type WriteCounter struct {
	Total uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc WriteCounter) PrintProgress() {
	fmt.Printf("\r%s", strings.Repeat(" ", 35))
	fmt.Printf("\rDownloading... %d B complete", wc.Total)
}

func DownloadFile(filepath string, url string) error {
	out, err := os.Create(filepath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	counter := &WriteCounter{}
	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return err
	}

	fmt.Print("\n")

	err = os.Rename(filepath+".tmp", filepath)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}

func getFileSize(s []int64) int64 {
	var sum int64 = 0
	for _, v := range s {
		sum += v
	}
	return sum
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	var vi VideoInfo
	var mi MediaInfo
	var cmd *exec.Cmd
	var stdout []byte
	var isNull int

	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")

	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	_ = cmdDetail.Run()
	if err != nil {
		log.Printf("download info failed!!!%v", err)
		mi.ErrCode = CanNotGetMediaInfo
		goto RESP
	}
	_ = json.Unmarshal(out, &vi)
	log.Printf("%v", vi)

	cmd = exec.Command("youtube-dl", "-g", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL)
	stdout, err = cmd.CombinedOutput()
	if err != nil {
		log.Print("命令输出到管道失败", err)
	}
	//defer stdout.Close()
	err = cmd.Run()
	if err != nil {
		log.Print("命令执行失败错误", err)
	}
	isNull =len(stdout)
	if isNull == 0 {
		log.Print("读取命令执行结果错误", err)
	} else {
		log.Println("获取的实际视频音频地址是", string(stdout))
		s := strings.Split(string(stdout), "\n")
		for i := 0; i < 2; i++ {
			if i == 0 {
				fmt.Println("开始下载视频文件......")
				err := DownloadFile("/data/youtube-dl/"+vi.Title+".mp4", s[0])
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Println("开始下载音频文件......")
				err := DownloadFile("/data/youtube-dl/"+vi.Title+".mp3", s[1])
				if err != nil {
					panic(err)
				}
			}
		}
	}

	//if vi.Extractor == "youtube" {
	//	switch mediaFormat {
	//	case "mp4":
	//		cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp4")
	//	default:
	//		cmd = exec.Command("youtube-dl", "-x", "--audio-format", "mp3", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp3")
	//	}
	//} else if vi.Extractor == "BiliBili" {
	//	switch mediaFormat {
	//	case "mp4":
	//		cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+"."+vi.Ext)
	//	default:
	//		cmd = exec.Command("youtube-dl", "-x", "--audio-format", "m4a", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".m4a")
	//	}
	//}
	//cmdDetail.Stdout = os.Stdout
	//cmdDetail.Stderr = os.Stderr
	//err = cmd.Run()
	//if err != nil {
	//	log.Printf("命令执行有错误%v", err)
	//	mi.ErrCode = YoutubeDLCommandError
	//	goto RESP
	//}
	//log.Printf("%v", vi)

	mi.VideoInfo = vi
	//if vi.Extractor == "BiliBili" {
	//	switch mediaFormat {
	//	case "mp4":
	//		mi.DownloadUrl = "/youtube-dl/" + vi.Title + "." + vi.Ext
	//	default:
	//		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".m4a"
	//	}
	//} else if vi.Extractor == "youtube" {
	switch mediaFormat {
	case "mp4":
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
	default:
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
	}
	//}

RESP:
	rsp, _ := json.Marshal(mi)
	//_, _ = io.WriteString(w, youtubeURL+"  "+mediaFormat)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}
