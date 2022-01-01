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
			want:    []Bencode{&IntElement{Value: 45902}},
			wantErr: false,
		},
		{
			name: "parse string",
			args: args{
				data: "5:hello",
			},
			want:    []Bencode{&StringElement{Value: "hello"}},
			wantErr: false,
		},
		{
			name: "parse list",
			args: args{
				data: "li45902e5:helloe",
			},
			want:    []Bencode{&ListElement{List: []Bencode{&IntElement{Value: 45902}, &StringElement{Value: "hello"}}}},
			wantErr: false,
		},
		{
			name: "parse dict",
			args: args{
				data: "d5:helloi45902e5:world2:mee",
			},
			want: []Bencode{&DictElement{Dict: map[StringElement]Bencode{
				{Value: "hello"}: &IntElement{Value: 45902},
				{Value: "world"}: &StringElement{Value: "me"}}}},
			wantErr: false,
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
