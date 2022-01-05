package main

import (
	"os"

	"os/signal"
	"syscall"

	"io/ioutil"

	"flag"

	"os/user"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/gotit"
	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	log "github.com/sirupsen/logrus"
)

var (
	torrentPath    = flag.String("file", "", "Path to torrent file")
	downloadFolder = flag.String("out", "", "Path to download location")
	listenPort     = flag.Uint("port", 8999, "Port used for listening incoming peer requests")
	logLevel       = flag.String("log-level", "fatal", "Log level for printing messages to console")
	peerNum        = flag.Int("peer-num", 100, "Number of concurrent peer download connections")
)

// set up logger
func initLogger() {
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		level = log.FatalLevel
	}
	log.SetOutput(os.Stdout)
	log.SetLevel(level)

	formatter := runtime.Formatter{
		ChildFormatter: &log.TextFormatter{
			FullTimestamp: true,
		},
	}
	log.SetFormatter(&formatter)
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

	torrentContent, _ := ioutil.ReadFile(*torrentPath)
	torrentString := string(torrentContent)
	benc, err := bencode.Parse(torrentString)
	if err != nil {
		log.Fatal("Error parsing torrent file: ", err)
	}

	// TODO: handle this better
	benDict := benc[0]
	log.Debug(benDict.String())
	dict, ok := benDict.(bencode.DictElement)
	if !ok {
		log.Fatal("Invalid torrent file")
	}

	// TODO: do we need to create torrent here just to pass it
	log.Info("Parsed torrent file")
	torrent := gotit.NewTorrent(dict)
	mng := gotit.NewMng(torrent, *peerNum)

	// TODO: this should be done by TorrentManager
	createTorrentFiles(torrent)

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		log.Info("Received signal. Exiting ...")
		mng.Close()
	}()

	if err := mng.Download(); err != nil {
		log.Fatal("Failed to download. Got error: ", err)
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
