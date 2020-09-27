package fileDownload

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
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
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}

var vi *VideoInfo
var mi MediaInfo
var cmd *exec.Cmd

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")
	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	if err != nil {
		log.Printf("download print-info failed%v", err)
		mi.ErrCode = CanNotGetMediaInfo
	}
	_ = json.Unmarshal(out, &vi)

	cmd = exec.Command("youtube-dl", "-g", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL)
	resp, err := cmd.CombinedOutput()
	if err != nil {
		log.Print("get the real file address failed", err)
	}
	var s []string
	s = strings.Split(string(resp), "\n")

	switch mediaFormat {
	case "mp4":
		log.Println("start downloading the video")
		fileDownload(s[0],vi.Title,"mp4")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
	default:
		log.Println("start downloading the audio")
		fileDownload(s[1],vi.Title,"mp3")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
	}

	rsp, _ := json.Marshal(mi)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}
