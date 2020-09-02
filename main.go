package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
)

type VideoInfo struct {
	UploadDate    string `json:"upload_date"`
	VideoDuration int    `json:"duration"`
	Title         string `json:"title"`
	Ext           string `json:"ext"`
	Uploader      string `json:"uploader"`
	Description   string `json:"description"`
}

type MediaUrl struct {
	VideoInfo   VideoInfo `json:"video_info"`
	DownloadUrl string    `json:"download_url"`
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}
func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")
	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	if err != nil {
		log.Printf("download info failed!!!%v", err)
	}
	var vi VideoInfo
	var mu MediaUrl
	var cmd *exec.Cmd
	_ = json.Unmarshal(out, &vi)

	switch mediaFormat {
	case "mp4":
		cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp4")
	default:
		cmd = exec.Command("youtube-dl", "-x", "--audio-format", "mp3", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+".mp3")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Printf("命令执行有错误%v", err)
	}
	log.Printf("%v", vi)
	mu.VideoInfo = vi
	mu.DownloadUrl = "http://shareforyou.online/youtube-dl/" + vi.Title + ".mp3"
	rsp, _ := json.Marshal(mu)
	//_, _ = io.WriteString(w, youtubeURL+"  "+mediaFormat)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}
