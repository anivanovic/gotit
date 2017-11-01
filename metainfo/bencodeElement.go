package metainfo

import (
	"fmt"
	"strconv"
	"strings"
)

type bencode interface {
	Type() string
	String() string
	Encode() string
}

type StringElement struct {
	value string
}

func (bencode StringElement) Type() string {
	return "String"
}

func (bencode StringElement) String() string {
	return bencode.value
}

func (bencode StringElement) Encode() string {
	return strconv.Itoa(len(bencode.value)) + ":" + bencode.value
}

type IntElement struct {
	value int
}

func (bencode IntElement) Type() string {
	return "Integer"
}

func (bencode IntElement) String() string {
	return strconv.Itoa(bencode.value)
}

func (bencode IntElement) Encode() string {
	return "i" + strconv.Itoa(bencode.value) + "e"
}

type ListElement struct {
	value []bencode
}

func (bencode ListElement) Type() string {
	return "List"
}

func (bencode ListElement) String() string {
	data := "["

	for i := 0; i < len(bencode.value); i++ {
		if bencode.value[i] == nil {
			fmt.Println("element nil: ", i)
		}
		data += bencode.value[i].String() + ", "
	}

	return data[:len(data)-2] + "]"
}

func (bencode ListElement) Encode() string {
	encoded := "l"
	for _, element := range bencode.value {
		encoded += element.Encode()
	}
	encoded += "e"

	return encoded
}

type DictElement struct {
	dict  map[StringElement]bencode
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
		fmt.Println(key)
		encoded += key.Encode()
		encoded += bencode.dict[key].Encode()
	}

	encoded += "e"

	return encoded
}

func (bencode DictElement) printValue(value bencode, tabs string) string {
	dict, ok := value.(DictElement)
	if ok {
		var data = "{\n"
		for _, k := range dict.order {
			data += tabs + k.String() + ": " + bencode.printValue(dict.dict[k], tabs+"\t") + "\n"
		}

		return data + "}\n"
	} else {
		return value.String()
	}
}

func (dict DictElement) Value(key string) bencode {
	keys := strings.Split(key, ".")

	var element bencode
	element = dict
	for _, key := range keys {
		benKey := StringElement{key}
		if castDict, ok := element.(DictElement); ok {
			element = castDict.dict[benKey]
		} else {
			element = nil
		}
	}

	return element
}
