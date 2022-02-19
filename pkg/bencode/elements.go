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
	data := "[\n"

	for _, el := range bencode {
		if el == nil {
			continue
		}
		data += "\t" + el.String() + ",\n"
	}

	return data + "]"
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
	return bencode.printValue(bencode, "\t")
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

func (bencode DictElement) printValue(value Bencode, tabs string) string {
	dict, ok := value.(DictElement)
	if ok {
		var data = "{\n"
		for k, v := range dict {
			if v == nil {
				continue
			}
			data += tabs + k + ": " + bencode.printValue(v, tabs+"\t") + ",\n"
		}

		return data + "}"
	} else {
		return value.String()
	}
}
