package main

import (

	// _ "net/http/pprof"
	"os"
	"syscall"

	"io/ioutil"

	"flag"

	"os/signal"
	"os/user"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/gotit"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	torrentPath    = flag.String("file", "", "Path to torrent file")
	downloadFolder = flag.String("out", "", "Path to download location")
	listenPort     = flag.Int("port", 8999, "Port used for listening incoming peer requests")
	logLevel       = flag.String("log-level", "fatal", "Log level for printing messages to console")
	peerNum        = flag.Int("peer-num", 100, "Number of concurrent peer download connections")
)

var log *zap.Logger

// set up logger
func initLogger() {
	l := zapcore.InfoLevel
	l.Set(*logLevel)

	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	cfg.Level = zap.NewAtomicLevelAt(l)
	log, _ = cfg.Build()

	zap.ReplaceGlobals(log)
	gotit.SetLogger(log)
}

func defaultDownloadFolder() string {
	user, _ := user.Current()
	defaultDownloadFolder := user.HomeDir + string(os.PathSeparator) + "Downloads"
	if _, err := os.Stat(defaultDownloadFolder); os.IsNotExist(err) {
		return user.HomeDir
	}

	return defaultDownloadFolder
}

func main() {
	flag.Parse()
	if *torrentPath == "" {
		flag.PrintDefaults()
		os.Exit(2)
	}
	if *downloadFolder == "" {
		*downloadFolder = defaultDownloadFolder()
	}
	initLogger()
	defer log.Sync()

	// go func() {
	// 	http.ListenAndServe("localhost:6060", nil)
	// }()

	data, err := ioutil.ReadFile(*torrentPath)
	if err != nil {
		log.Fatal("error reading torrent file", zap.String("file_path", *torrentPath), zap.Error(err))
	}
	torrentMeta := &gotit.TorrentMetadata{}
	if err := bencode.Unmarshal(data, torrentMeta); err != nil {
		log.Fatal("Error parsing torrent file", zap.Error(err))
	}
	log.Debug("torrent file", zap.Object("torrentMeta", torrentMeta))

	log.Info("Parsed torrent file")
	torrent := gotit.NewTorrent(torrentMeta)
	mng := gotit.NewMng(torrent, *peerNum, *listenPort)

	// TODO: this should be done by TorrentManager
	createTorrentFiles(torrent)

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		log.Info("Received signal. Exiting ...")
		mng.Close()
	}()

	log.Info("staring download")
	if err := mng.Download(); err != nil {
		log.Fatal("Torrent download error", zap.Error(err))
	}

	log.Info("Download finished")
}

func createTorrentFiles(torrent *gotit.Torrent) error {
	torrentDirPath := *downloadFolder + torrent.Name
	if torrent.IsDirectory {
		if err := os.Mkdir(torrentDirPath, os.ModeDir); err != nil {
			return err
		}

		for _, torrentFile := range torrent.TorrentFiles {
			file, err := os.Create(torrentDirPath + "/" + torrentFile.Path)
			if err != nil {
				return err
			}
			torrent.OsFiles = append(torrent.OsFiles, file)
		}
	} else {
		torrentFile, err := os.Create(torrentDirPath)
		if err != nil {
			return err
		}
		torrent.OsFiles = append(torrent.OsFiles, torrentFile)
	}

	return nil
}
