package metainfo

import (
	"fmt"
	"strconv"
	"strings"
)

type Bencode interface {
	Type() string
	String() string
	Encode() string
}

type StringElement struct {
	Value string
}

func (bencode StringElement) Type() string {
	return "String"
}

func (bencode StringElement) String() string {
	return bencode.Value
}

func (bencode StringElement) Encode() string {
	return strconv.Itoa(len(bencode.Value)) + ":" + bencode.Value
}

type IntElement struct {
	Value int
}

func (bencode IntElement) Type() string {
	return "Integer"
}

func (bencode IntElement) String() string {
	return strconv.Itoa(bencode.Value)
}

func (bencode IntElement) Encode() string {
	return "i" + strconv.Itoa(bencode.Value) + "e"
}

type ListElement struct {
	List []Bencode
}

func (bencode ListElement) Type() string {
	return "List"
}

func (bencode ListElement) String() string {
	data := "["

	for i := 0; i < len(bencode.List); i++ {
		if bencode.List[i] == nil {
			fmt.Println("element nil: ", i)
		}
		data += bencode.List[i].String() + ", "
	}

	return data[:len(data)-2] + "]"
}

func (bencode ListElement) Encode() string {
	encoded := "l"
	for _, element := range bencode.List {
		encoded += element.Encode()
	}
	encoded += "e"

	return encoded
}

type DictElement struct {
	Dict  map[StringElement]Bencode
	order []StringElement
}

func (bencode DictElement) Type() string {
	return "Dictionery"
}

func (bencode DictElement) String() string {
	return bencode.printValue(bencode, "\t")
}

func (bencode DictElement) Encode() string {
	encoded := "d"

	for _, key := range bencode.order {
		encoded += key.Encode()
		encoded += bencode.Dict[key].Encode()
	}

	encoded += "e"

	return encoded
}

func (bencode DictElement) printValue(value Bencode, tabs string) string {
	dict, ok := value.(DictElement)
	if ok {
		var data = "{\n"
		for _, k := range dict.order {
			data += tabs + k.String() + ": " + bencode.printValue(dict.Dict[k], tabs+"\t") + "\n"
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
		benKey := StringElement{key}
		if castDict, ok := element.(DictElement); ok {
			element = castDict.Dict[benKey]
		} else {
			element = nil
		}
	}

	return element
}
