package bencode

import (
	"fmt"
	"strconv"
	"strings"
)

type Bencode interface {
	String() string
	Encode() string
}

type StringElement string

func (bencode StringElement) String() string {
	return string(bencode)
}

func (bencode StringElement) Encode() string {
	return strconv.Itoa(len(bencode)) + ":" + string(bencode)
}

type IntElement int

func (bencode IntElement) String() string {
	return strconv.Itoa(int(bencode))
}

func (bencode IntElement) Encode() string {
	return "i" + strconv.Itoa(int(bencode)) + "e"
}

type ListElement []Bencode

func (bencode ListElement) String() string {
	data := "["

	for i := 0; i < len(bencode); i++ {
		if bencode[i] == nil {
			fmt.Println("element nil: ", i)
		}
		data += bencode[i].String() + ", "
	}

	return data[:len(data)-2] + "]"
}

func (bencode ListElement) Encode() string {
	encoded := "l"
	for _, element := range bencode {
		encoded += element.Encode()
	}
	return encoded + "e"
}

type DictElement map[StringElement]Bencode

func (bencode DictElement) String() string {
	return bencode.printValue(bencode, "\t")
}

func (bencode DictElement) Encode() string {
	encoded := "d"
	for k, v := range bencode {
		encoded += k.Encode()
		encoded += v.Encode()
	}
	encoded += "e"

	return encoded
}

func (bencode DictElement) printValue(value Bencode, tabs string) string {
	dict, ok := value.(DictElement)
	if ok {
		var data = "{\n"
		for k, v := range dict {
			data += tabs + k.String() + ": " + bencode.printValue(v, tabs+"\t") + "\n"
		}

		return data + "}\n"
	} else {
		return value.String()
	}
}

func (dict DictElement) Value(key string) Bencode {
	keys := strings.Split(key, ".")

	var element Bencode
	element = dict
	for _, key := range keys {
		benKey := StringElement(key)
		if castDict, ok := element.(DictElement); ok {
			element = castDict[benKey]
		} else {
			element = nil
		}
	}

	return element
}
