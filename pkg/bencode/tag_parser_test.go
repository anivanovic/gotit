package bencode_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anivanovic/gotit/pkg/bencode"
)

func readTorrentFile(t testing.TB, name string) []byte {
	if t != nil {
		t.Helper()
	}

	f, err := os.Open(fmt.Sprintf("./testdata/%s", name))
	if err != nil {
		t.Fatal("reading torrent file:", err)
	}
	data, _ := io.ReadAll(f)
	return data
}

func TestUnmarshal(t *testing.T) {
	data := readTorrentFile(t, "tears-of-steel.torrent")
	torrent := &bencode.Metainfo{}
	err := bencode.Unmarshal(data, torrent)
	assert.NoError(t, err)

	assert.Equal(t, "udp://tracker.leechers-paradise.org:6969", torrent.Announce)
	assert.Equal(t, "WebTorrent <https://webtorrent.io>", torrent.Comment)
	assert.Equal(t, "WebTorrent <https://webtorrent.io>", torrent.CreatedBy)
	assert.Equal(t, int64(1490916654), torrent.CreationDate)
	assert.Len(t, torrent.AnnounceList, 8)
	assert.Len(t, torrent.UrlList, 1)
	assert.NotNil(t, torrent.Info)
	info := torrent.Info
	assert.Equal(t, "Tears of Steel", info.Name)
	assert.Equal(t, int64(0), info.Length)
	assert.Equal(t, int64(524288), info.PieceLength)
	assert.NotEmpty(t, info.Pieces)
	assert.NotEmpty(t, info.Files)
	assert.NotNil(t, torrent.InfoDictRaw)
	files := info.Files
	for _, f := range files {
		assert.NotZero(t, f.Length)
		assert.NotEmpty(t, f.Path)
	}

	assert.Equal(
		t,
		torrent.String(),
		`{
	info: {
		name: Tears of Steel
		length: 0B
		piece-length: 524288
		files: [
			[path: Tears of Steel.de.srt, size: 4.7K],
			[path: Tears of Steel.en.srt, size: 4.6K],
			[path: Tears of Steel.es.srt, size: 4.8K],
			[path: Tears of Steel.fr.srt, size: 4.5K],
			[path: Tears of Steel.it.srt, size: 4.6K],
			[path: Tears of Steel.nl.srt, size: 4.4K],
			[path: Tears of Steel.no.srt, size: 9.3K],
			[path: Tears of Steel.ru.srt, size: 5.8K],
			[path: Tears of Steel.webm, size: 544.9M],
			[path: poster.jpg, size: 35.2K],
		]
	}
	comment: WebTorrent <https://webtorrent.io>
	created-by: WebTorrent <https://webtorrent.io>
	creation-date: 2017-03-31 00:30:54
	announce: udp://tracker.leechers-paradise.org:6969
	announce-list: [
		[udp://tracker.leechers-paradise.org:6969]
		[udp://tracker.coppersurfer.tk:6969]
		[udp://tracker.opentrackr.org:1337]
		[udp://explodie.org:6969]
		[udp://tracker.empire-js.us:1337]
		[wss://tracker.btorrent.xyz]
		[wss://tracker.openwebtorrent.com]
		[wss://tracker.fastcast.nz]
	]
	url-list: [
		https://webtorrent.io/torrents/,
	]
	(calculated) info-hash: IJyCJrKZswi-ryuc0_tJIS29E-w=
}
`)
}

func TestUnmarshal_TargetNotPointer(t *testing.T) {
	t.Parallel()

	data := readTorrentFile(t, "tears-of-steel.torrent")

	target := bencode.TorrentFile{}
	err := bencode.Unmarshal(data, target)
	assert.Error(t, err)
}

func TestUnmarshal_TargetNotPointerStruct(t *testing.T) {
	t.Parallel()

	data := readTorrentFile(t, "tears-of-steel.torrent")

	target := []string{}
	err := bencode.Unmarshal(data, target)
	assert.Error(t, err)
}

func TestUnmarshal_TypeError(t *testing.T) {
	data := bencode.NewDictBuilder().
		Add("int", bencode.List(bencode.String("string_value"))).
		Generate().
		Encode()
	type testStruct struct {
		IntValue int `ben:"int"`
	}
	target := &testStruct{}
	err := bencode.Unmarshal([]byte(data), target)
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "could not assign bencode value to target (field name: IntValue, field type: int, bencode type: *bencode.ListElement)")
}

func TestUnmarshal_SupportedTypes(t *testing.T) {
	type testStruct struct {
		Uint8  uint8  `ben:"uint8"`
		Uint16 uint16 `ben:"uint16"`
		Uint32 uint32 `ben:"uint32"`
		Uint64 uint64 `ben:"uint64"`

		Int   int   `ben:"int"`
		Int8  int8  `ben:"int8"`
		Int16 int16 `ben:"int16"`
		Int32 int32 `ben:"int32"`
		Int64 int64 `ben:"int64"`

		MapOfLists map[string][]string `ben:"map_lists"`
		Slice      []string            `ben:"string_list"`
		Struct     struct {
			InsideInt int `ben:"inside_int"`
		} `ben:"struct"`
		StructPointer *struct {
			InsideInt int `ben:"inside_int"`
		} `ben:"struct_pointer"`
		IntPointer *int `ben:"int_pointer"`
	}
	bencodeValue := bencode.NewDictBuilder().
		Add("uint8", bencode.Integer(8)).
		Add("uint16", bencode.Integer(16)).
		Add("uint32", bencode.Integer(32)).
		Add("uint64", bencode.Integer(64)).
		Add("int", bencode.Integer(1)).
		Add("int8", bencode.Integer(8)).
		Add("int16", bencode.Integer(16)).
		Add("int32", bencode.Integer(32)).
		Add("int64", bencode.Integer(64)).
		Add("map_lists", bencode.NewDictBuilder().
			Add("list", bencode.List(bencode.String("string list value"))).
			Generate()).
		Add("string_list", bencode.List(bencode.String("first"), bencode.String("second"), bencode.String("third"))).
		Add("struct", bencode.NewDictBuilder().Add("inside_int", bencode.Integer(11)).Generate()).
		Add("struct_pointer", bencode.NewDictBuilder().Add("inside_int", bencode.Integer(12)).Generate()).
		Add("int_pointer", bencode.Integer(33)).
		Generate().
		Encode()

	target := &testStruct{}
	err := bencode.Unmarshal([]byte(bencodeValue), target)
	assert.NoError(t, err)
	assert.Equal(t, target.Uint8, uint8(8))
	assert.Equal(t, target.Uint16, uint16(16))
	assert.Equal(t, target.Uint32, uint32(32))
	assert.Equal(t, target.Uint64, uint64(64))
	assert.Equal(t, target.Int, 1)
	assert.Equal(t, target.Int8, int8(8))
	assert.Equal(t, target.Int16, int16(16))
	assert.Equal(t, target.Int32, int32(32))
	assert.Equal(t, target.Int64, int64(64))
	assert.NotEmpty(t, target.MapOfLists)
	assert.NotNil(t, target.MapOfLists["list"])
	assert.Contains(t, target.MapOfLists["list"], "string list value")
	assert.NotNil(t, target.Slice)
	assert.ElementsMatch(t, target.Slice, []string{"first", "second", "third"})
	assert.Equal(t, target.Struct.InsideInt, 11)
	assert.NotNil(t, target.StructPointer)
	assert.Equal(t, target.StructPointer.InsideInt, 12)
	assert.Equal(t, *target.IntPointer, 33)
}

func BenchmarkUnmarshal(b *testing.B) {
	data := readTorrentFile(b, "tears-of-steel.torrent")
	torrent := bencode.TorrentFile{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bencode.Unmarshal(data, &torrent); err != nil {
			b.Fatal(err)
		}
	}
}
