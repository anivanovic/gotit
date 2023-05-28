package bencode

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestParse(t *testing.T) {
	var nilDict *DictElement
	var nilList *ListElement
	type args struct {
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    Bencode
		wantErr bool
	}{
		{
			name: "empty bencode",
			args: args{
				data: "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parse integer",
			args: args{
				data: "i45902e",
			},
			want:    IntElement(45902),
			wantErr: false,
		},
		{
			name: "parse string",
			args: args{
				data: "5:hello",
			},
			want:    StringElement("hello"),
			wantErr: false,
		},
		{
			name: "parse list",
			args: args{
				data: "li45902e5:helloe",
			},
			want: &ListElement{
				Value: []Bencode{IntElement(45902), StringElement("hello")},
				raw:   []byte("li45902e5:helloe"),
			},
			wantErr: false,
		},
		{
			name: "parse dict",
			args: args{
				data: "d5:helloi45902e5:world2:mee",
			},
			want: &DictElement{
				value: map[string]Bencode{
					"hello": IntElement(45902),
					"world": StringElement("me")},
				raw: []byte("d5:helloi45902e5:world2:mee"),
			},
			wantErr: false,
		},
		{
			name: "string len error",
			args: args{
				data: "6:hello",
			},
			want:    StringElement(""),
			wantErr: true,
		},
		{
			name: "integer not ended",
			args: args{
				data: "i45",
			},
			want:    IntElement(0),
			wantErr: true,
		},
		{
			name: "list not ended",
			args: args{
				data: "li42ei22e",
			},
			want:    nilList,
			wantErr: true,
		},
		{
			name: "dict not ended",
			args: args{
				data: "d5:firsti45",
			},
			want:    nilDict,
			wantErr: true,
		},
		{
			name: "string len error in middle of dict",
			args: args{
				data: "d2:firsti45e",
			},
			want:    nilDict,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.args.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

var data = readTorrentFile(nil, "ubuntu-21.04-desktop-amd64.iso.torrent")

func BenchmarkParse(b *testing.B) {
	var r Bencode
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err = Parse(data)
	}
	b.StopTimer()
	assert.NotNil(b, r)
	assert.NoError(b, err)
}
