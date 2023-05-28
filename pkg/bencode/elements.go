package bencode

import (
	"strconv"
)

type (
	Bencode interface {
		String() string
		Encode() string
		Raw() []byte
	}

	IntElement    int
	StringElement string
	ListElement   struct {
		Value []Bencode
		raw   []byte
	}
	DictElement struct {
		value map[string]Bencode
		raw   []byte
	}
)

func (bencode StringElement) String() string {
	return string(bencode)
}

func (bencode StringElement) Encode() string {
	return strconv.Itoa(len(bencode)) + ":" + bencode.String()
}

func (bencode StringElement) Raw() []byte {
	return []byte(bencode)
}

func (bencode IntElement) String() string {
	return strconv.Itoa(int(bencode))
}

func (bencode IntElement) Encode() string {
	return "i" + bencode.String() + "e"
}

func (bencode IntElement) Raw() []byte {
	return []byte(bencode.String())
}

func (bencode ListElement) String() string {
	return prettyPrint(bencode, "")
}

func (bencode ListElement) Encode() string {
	encoded := "l"
	for _, el := range bencode.Value {
		if el == nil {
			continue
		}
		encoded += el.Encode()
	}
	return encoded + "e"
}

func (bencode ListElement) Raw() []byte {
	return bencode.raw
}

func (bencode DictElement) String() string {
	return prettyPrint(bencode, "")
}

func (bencode DictElement) Encode() string {
	encoded := "d"
	for k, v := range bencode.value {
		if v == nil {
			continue
		}
		encoded += StringElement(k).Encode()
		encoded += v.Encode()
	}

	return encoded + "e"
}

func (bencode DictElement) Value(key string) Bencode {
	return bencode.value[key]
}

func (bencode DictElement) Raw() []byte {
	return bencode.raw
}

func prettyPrint(bencode Bencode, tabs string) string {
	switch value := bencode.(type) {
	case DictElement:
		tabs = addTab(tabs)
		data := "{" + newLine(tabs)

		for k, v := range value.value {
			if v == nil {
				continue
			}
			data += k + ": " + prettyPrint(v, tabs) + "," + newLine(tabs)
		}

		if len(data) == 2+len(tabs) {
			return "{}"
		}

		return data[:len(data)-1] + "}"
	case ListElement:
		tabs = addTab(tabs)
		data := "[" + newLine(tabs)
		for _, el := range value.Value {
			if el == nil {
				continue
			}
			data += prettyPrint(el, tabs) + "," + newLine(tabs)
		}

		if len(data) == 2+len(tabs) {
			return "[]"
		}

		return data[:len(data)-1] + "]"
	default:
		return value.String()
	}
}

func addTab(tabs string) string {
	return tabs + "\t"
}

func newLine(tabs string) string {
	return "\n" + tabs
}

type (
	dictBencodeBuilder struct {
		dict map[string]Bencode
	}
	Builder interface {
		Add(key string, value Bencode) Builder
		Generate() Bencode
	}
)

func (d *dictBencodeBuilder) Add(key string, value Bencode) Builder {
	d.dict[key] = value
	return d
}

func (d *dictBencodeBuilder) Generate() Bencode {
	return DictElement{value: d.dict}
}

func NewDictBuilder() Builder {
	return &dictBencodeBuilder{dict: map[string]Bencode{}}
}

func String(value string) Bencode {
	return StringElement(value)
}

func Integer(value int) Bencode {
	return IntElement(value)
}

func List(values ...Bencode) Bencode {
	return ListElement{Value: values}
}
