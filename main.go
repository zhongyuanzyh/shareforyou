package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	VideoInfo   VideoInfo `json:"video_info"`
	DownloadUrl string    `json:"download_url"`
	ErrCode     int       `json:"error_code"`
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}
func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	var vi VideoInfo
	var mi MediaInfo
	var cmd *exec.Cmd

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
			cmd = exec.Command("youtube-dl", "-x", "--audio-format", "mp3", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp3")
		}
	} else if vi.Extractor == "BiliBili" {
		switch mediaFormat {
		case "mp4":
			cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+vi.Ext)
		default:
			cmd = exec.Command("youtube-dl", "-x", "--audio-format", "m4a", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".m4a")
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Printf("命令执行有错误%v", err)
		mi.ErrCode = YoutubeDLCommandError
		goto RESP
	}
	log.Printf("%v", vi)
	mi.VideoInfo = vi
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
