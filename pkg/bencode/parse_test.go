package bencode

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    []Bencode
		wantErr bool
	}{
		{
			name: "empty bencode",
			args: args{
				data: "",
			},
			want:    []Bencode{},
			wantErr: false,
		},
		{
			name: "parse integer",
			args: args{
				data: "i45902e",
			},
			want:    []Bencode{IntElement(45902)},
			wantErr: false,
		},
		{
			name: "parse string",
			args: args{
				data: "5:hello",
			},
			want:    []Bencode{StringElement("hello")},
			wantErr: false,
		},
		{
			name: "parse list",
			args: args{
				data: "li45902e5:helloe",
			},
			want:    []Bencode{ListElement([]Bencode{IntElement(45902), StringElement("hello")})},
			wantErr: false,
		},
		{
			name: "parse dict",
			args: args{
				data: "d5:helloi45902e5:world2:mee",
			},
			want: []Bencode{DictElement(map[StringElement]Bencode{
				"hello": IntElement(45902),
				"world": StringElement("me")})},
			wantErr: false,
		},
		{
			name: "string len error",
			args: args{
				data: "6:hello",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "integer not ended",
			args: args{
				data: "i45",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "list not ended",
			args: args{
				data: "li42ei22e",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "dict not ended",
			args: args{
				data: "d5:firsti45",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "string len error in middle of dict",
			args: args{
				data: "d2:firsti45e",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args.data)
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
