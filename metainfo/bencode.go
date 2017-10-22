package metainfo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func CheckError(e error) {
	if e != nil {
		panic(e)
	} else {
		fmt.Println("nothing here")
	}
}

func Decode(data string) (string, bencode) {
	startTag := string(data[0])

	switch startTag {
	case "l":
		return readList(data)
	case "d":
		return readDict(data)
	case "i":
		return readInt(data)
	default:
		return readString(data)
	}
}

func Parse(data string) (error, *DictElement) {
	_, bencode := Decode(data)

	if benDict, ok := bencode.(DictElement); ok {
		return nil, &benDict
	} else {
		return errors.New("Invalid torrent file"), nil
	}
}

func readInt(data string) (string, IntElement) {
	valueEndIndex := strings.Index(data, "e")
	value := data[1:valueEndIndex]
	intVal, _ := strconv.Atoi(value)

	i := IntElement{value: intVal}

	return data[valueEndIndex+1:], i
}

func readString(data string) (string, StringElement) {
	stringValueIndex := strings.Index(data, ":") + 1

	valueLen, _ := strconv.Atoi(data[:stringValueIndex-1])
	value := data[stringValueIndex : stringValueIndex+valueLen]
	s := StringElement{value: value}

	return data[stringValueIndex+valueLen:], s
}

func readList(data string) (string, ListElement) {
	data = data[1:] // remove first l

	bencodeList := make([]bencode, 0)
	var element bencode
	for strings.Index(data, "e") != 0 {
		data, element = Decode(data)
		if element == nil {
			fmt.Println("citanje nil element")
		}
		bencodeList = append(bencodeList, element)
	}

	return data[1:], ListElement{elements: bencodeList}
}

func readDict(data string) (string, DictElement) {
	data = data[1:] // remove firs d

	dict := make(map[StringElement]bencode)
	var k StringElement
	var v bencode
	for strings.Index(data, "e") != 0 {
		data, k = readString(data)
		data, v = Decode(data)
		dict[k] = v
	}

	return data[1:], DictElement{dict: dict}
}
