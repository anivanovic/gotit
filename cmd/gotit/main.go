package main

import (

	// _ "net/http/pprof"
	"os"
	"path/filepath"
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
	torrentPath    = flag.String("torrent", "", "Path to torrent file")
	downloadFolder = flag.String("out", "", "Path to download location")
	listenPort     = flag.Int("port", 8999, "Port used for listening incoming peer requests")
	logLevel       = flag.String("log-level", "fatal", "Log level for printing messages to console")
	peerNum        = flag.Int("peer-num", 30, "Number of concurrent peer download connections")
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

	defaultDownloadFolder := filepath.Join(user.HomeDir, "Downloads")
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

	initLogger()
	defer log.Sync()

	if _, err := os.Stat(*torrentPath); os.IsNotExist(err) {
		log.Fatal("Torrent file does not exist", zap.String("path", *torrentPath), zap.Error(err))
	}
	if *downloadFolder == "" {
		*downloadFolder = defaultDownloadFolder()
	}
	if _, err := os.Stat(*downloadFolder); os.IsNotExist(err) {
		log.Fatal("Download folder does not exist", zap.String("path", *downloadFolder), zap.Error(err))
	}

	// go func() {
	// 	http.ListenAndServe("localhost:6060", nil)
	// }()

	data, err := ioutil.ReadFile(*torrentPath)
	if err != nil {
		log.Fatal("error reading torrent file", zap.String("path", *torrentPath), zap.Error(err))
	}
	torrentMeta := &gotit.TorrentMetadata{}
	if err := bencode.Unmarshal(data, torrentMeta); err != nil {
		log.Fatal("Error parsing torrent file", zap.Error(err))
	}
	log.Debug("torrent file", zap.Object("torrentMeta", torrentMeta))

	log.Info("Torrent file parsed")
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
	path := filepath.Join(*downloadFolder, torrent.Name)
	var filePaths []string
	if torrent.IsDirectory {
		if err := os.Mkdir(path, os.ModeDir); err != nil {
			return err
		}

		for _, tf := range torrent.TorrentFiles {
			filePaths = append(filePaths, filepath.Join(path, tf.Path))
		}
	} else {
		filePaths = append(filePaths, path)
	}

	for _, path := range filePaths {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		torrent.OsFiles = append(torrent.OsFiles, f)
	}

	return nil
}
