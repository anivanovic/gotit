package bencode

import (
	"strconv"
)

type (
	Bencode interface {
		String() string
		Encode() string
	}

	IntElement    int
	StringElement string
	ListElement   []Bencode
	DictElement   map[string]Bencode
)

func (bencode StringElement) String() string {
	return string(bencode)
}

func (bencode StringElement) Encode() string {
	return strconv.Itoa(len(bencode)) + ":" + bencode.String()
}

func (bencode IntElement) String() string {
	return strconv.Itoa(int(bencode))
}

func (bencode IntElement) Encode() string {
	return "i" + bencode.String() + "e"
}

func (bencode ListElement) String() string {
	return prettyPrint(bencode, "")
}

func (bencode ListElement) Encode() string {
	encoded := "l"
	for _, el := range bencode {
		if el == nil {
			continue
		}
		encoded += el.Encode()
	}
	return encoded + "e"
}

func (bencode DictElement) String() string {
	return prettyPrint(bencode, "")
}

func (bencode DictElement) Encode() string {
	encoded := "d"
	for k, v := range bencode {
		if v == nil {
			continue
		}
		encoded += StringElement(k).Encode()
		encoded += v.Encode()
	}

	return encoded + "e"
}

func (dict DictElement) Value(key string) Bencode {
	return dict[key]
}

func prettyPrint(value Bencode, tabs string) string {
	switch value := value.(type) {
	case DictElement:
		tabs = addTab(tabs)
		data := "{" + newLine(tabs)

		for k, v := range value {
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
		for _, el := range value {
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
