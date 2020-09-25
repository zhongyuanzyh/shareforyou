package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const (
	CanNotGetMediaInfo    = 101
	YoutubeDLCommandError = 102
	ConvertSuccess        = 103
)

type VideoInfo struct {
	UploadDate       string        `json:"upload_date"`
	VideoDuration    int           `json:"duration"`
	Title            string        `json:"title"`
	Ext              string        `json:"ext"`
	Uploader         string        `json:"uploader"`
	Description      string        `json:"description"`
	Extractor        string        `json:"extractor"`
	RequestedFormats [2]FormatInfo `json:"requested_formats"`
}
type FormatInfo struct {
	FileSize  int    `json:"filesize"`
	FormatID  string `json:"format_id"`
	Extension string `json:"ext"`
}

type MediaInfo struct {
	VideoInfo        VideoInfo `json:"video_info"`
	DownloadUrl      string    `json:"download_url"`
	DownloadProgress float64   `json:"download_progress"`
	ErrCode          int       `json:"error_code"`
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	http.HandleFunc("/progress", youtubeProgress)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	mux.HandleFunc("/progress", youtubeProgress)
	_ = http.ListenAndServe(":8888", mux)
}

var vi *VideoInfo
var mi MediaInfo
var cmd *exec.Cmd

func youtubeProgress(w http.ResponseWriter, r *http.Request) {
	//type A struct {
	//	P float64 `json:"progress"`
	//	D string  `json:"link"`
	//	T string  `json:"title"`
	//}
	//var rp A
	//rp.P = mi.DownloadProgress
	//rp.D = mi.DownloadUrl
	//rp.T = mi.VideoInfo.Title
	rsp, _ := json.Marshal(mi)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
	mi.DownloadProgress = 0
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")
	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	if err != nil {
		log.Printf("download info failed!!!%v", err)
		mi.ErrCode = CanNotGetMediaInfo
		goto RESP
	}

	_ = json.Unmarshal(out, &vi)
	if vi.Extractor == "youtube" {
		switch mediaFormat {
		case "mp4":
			cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp4")
		default:
			cmd = exec.Command("youtube-dl", "-x", "--audio-format", "mp3", "-r", "800K", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp3")
		}
	} else if vi.Extractor == "BiliBili" {
		switch mediaFormat {
		case "mp4":
			cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+"."+vi.Ext)
		default:
			cmd = exec.Command("youtube-dl", "-x", "--audio-format", "m4a", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".m4a")
		}
	}

	go func() {
		err = cmd.Run()
		if err != nil {
			log.Printf("命令执行有错误%v", err)
			mi.ErrCode = YoutubeDLCommandError
			goto RESP2
		}
	RESP2:
		rsp, _ := json.Marshal(mi)
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(rsp)
		return
	}()

	go func() {
		for {
			fi, err := os.Stat("/data/youtube-dl/" + vi.Title + ".mp3" + ".part")
			if err != nil {
				fmt.Println("文件信息报错", err)
			} else {
				fmt.Println("json获取的文件大小是：", vi.RequestedFormats[0].FileSize, vi.RequestedFormats[1].FileSize)
				fmt.Printf("文件下载百分比是:%.2f\n", float64(fi.Size())/float64(vi.RequestedFormats[1].FileSize)*100)
				mi.DownloadProgress = float64(fi.Size()) / float64(vi.RequestedFormats[1].FileSize) * 100
				time.Sleep(time.Duration(1000) * time.Millisecond)
			}
		}
	}()

	mi.VideoInfo = *vi
	if vi.Extractor == "BiliBili" {
		switch mediaFormat {
		case "mp4":
			mi.DownloadUrl = "/youtube-dl/" + vi.Title + "." + vi.Ext
		default:
			mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".m4a"
		}
	} else if vi.Extractor == "youtube" {
		switch mediaFormat {
		case "mp4":
			mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
		default:
			mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
		}
	}

RESP:
	rsp, _ := json.Marshal(mi)
	//_, _ = io.WriteString(w, youtubeURL+"  "+mediaFormat)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}
