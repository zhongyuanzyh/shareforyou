package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

var vi VideoInfo
var mi MediaInfo
var cmd *exec.Cmd

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

func timerTask(second int, f func()) {
	timer1 := time.NewTicker(time.Duration(second) * time.Second)
	for {
		select {
		case <-timer1.C:
			f()
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func monitorFileSize() {
	if vi.VideoDuration != 0 {
		log.Print("file coming...")
		fs := getFileSize(mi.FileSize)
		fi, _ := os.Stat("/data/youtube-dl/" + vi.Title + ".mp3")
		log.Printf("获取的大小分别是：%v---%v",fs,fi)
		//fi,_:=os.Stat("/data/youtube-dl/"+vi.Title+".m4a")
		progressRation := fi.Size() / fs * 100
		//w.Header().Add("Content-Type", "application/json; charset=utf-8")
		//_, _ = w.Write([]byte(string(progressRation)))
		log.Print(progressRation)
	} else {
		log.Print("还没有拿到文件的时长")
	}
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	go timerTask(1, monitorFileSize)

	//go func(){
	//	for {
	//		if vi.VideoDuration != 0 {
	//			log.Print("file coming...")
	//			fs := getFileSize(mi.FileSize)
	//			fi,_ :=os.Stat("/data/youtube-dl/"+vi.Title+".mp3")
	//			//fi,_:=os.Stat("/data/youtube-dl/"+vi.Title+".m4a")
	//			progressRation := fi.Size()/fs * 100
	//			w.Header().Add("Content-Type", "application/json; charset=utf-8")
	//			_,_ = w.Write([]byte(string(progressRation)))
	//		}else{
	//			log.Print("还没有拿到文件的时长")
	//		}
	//	}
	//}()

	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")

	cmd = exec.Command("youtube-dl", "-g", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print("命令输出到管道失败", err)
	}
	defer stdout.Close()
	err = cmd.Start()
	if err != nil {
		log.Print("命令执行失败错误", err)
	}
	opBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		log.Print("读取命令执行结果错误", err)
	} else {
		log.Println("获取的实际视频音频地址是", string(opBytes))
		s := strings.Split(string(opBytes), "\n")
		client := new(http.Client)
		for i := 0; i < 2; i++ {
			resp, err := client.Get(s[i])
			if err != nil {
				log.Print("获取文件信息失败", err)
			}
			var fsize int64
			fsize, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 32)
			if err != nil {
				log.Print("获取文件大小失败", err)
			}
			log.Print("文件大小是：", fsize)
			mi.FileSize = append(mi.FileSize, fsize)
			for _, v := range mi.FileSize {
				log.Print("获取的文件大小是", v)
			}
			resp.Body.Close()
		}
	}

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
			cmd = exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL, "-o", "/data/youtube-dl/"+vi.Title+"."+vi.Ext)
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
