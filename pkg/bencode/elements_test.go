package bencode

import (
	"testing"
)

func TestBencode_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		bencode Bencode
		want    string
	}{
		{
			name:    "Empty string returns empty",
			bencode: StringElement(""),
			want:    "",
		},
		{
			name:    "String with content",
			bencode: StringElement("My string"),
			want:    "My string",
		},
		{
			name:    "Integer string",
			bencode: IntElement(5),
			want:    "5",
		},
		{
			name:    "List string",
			bencode: ListElement{Value: []Bencode{IntElement(5), nil}},
			want:    "[\n\t5,\n]",
		},
		{
			name: "Dict string",
			bencode: DictElement{
				value: map[string]Bencode{
					"key":  StringElement("Value"),
					"data": nil,
				}},
			want: "{\n\tkey: Value,\n}",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.bencode.String(); got != tt.want {
				t.Errorf("StringElement.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBencode_Encode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		bencode Bencode
		want    string
	}{
		{
			name:    "Empty string encoded",
			bencode: StringElement(""),
			want:    "0:",
		},
		{
			name:    "String encoded",
			bencode: StringElement("My string"),
			want:    "9:My string",
		},
		{
			name:    "Int encoded",
			bencode: IntElement(123),
			want:    "i123e",
		},
		{
			name:    "List encoded",
			bencode: ListElement{Value: []Bencode{StringElement("string"), IntElement(1), nil}},
			want:    "l6:stringi1ee",
		},
		{
			name: "Dict encoded",
			bencode: DictElement{
				value: map[string]Bencode{
					"key":  StringElement("value"),
					"list": ListElement{Value: []Bencode{IntElement(12)}},
					"nil":  nil,
				}},
			want: "d3:key5:value4:listli12eee",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.bencode.Encode(); got != tt.want {
				t.Errorf("StringElement.Encode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrettyPrint(t *testing.T) {
	t.Parallel()

	bencode := ListElement{
		Value: []Bencode{DictElement{
			value: map[string]Bencode{
				"1": StringElement("Value"),
				"2": IntElement(1),
				"3": ListElement{Value: []Bencode{StringElement("string")}},
				"4": DictElement{},
			}},
			StringElement("string")},
	}
	expected := `[
	{
		1: Value,
		2: 1,
		3: [
			string,
		],
		4: {},
	},
	string,
]`

	got := bencode.String()
	if got != expected {
		t.Errorf("Bencode does not print pretty: got = %v, expected = %v", got, expected)
	}
}
