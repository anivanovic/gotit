package metainfo

import (
	"fmt"
	"strings"
	"strconv"
)

func CheckError(e error) {
	if e != nil {
		panic(e)
	} else {
		fmt.Println("nothing here")
	}
}


func Decode(data string, printStr string, tab string) (string, string) {
	startTag := string(data[0])
	
	switch startTag {
	case "l":
		return readList(data, printStr, tab)
	case "d":
		return readDict(data, printStr, tab)
	case "i":
		return readInt(data, printStr)
	default:
		return readString(data, printStr)
	}
}

func readInt(data string, printStr string) (string, string) {
	valueEndIndex := strings.Index(data, "e")
	value := data[1:valueEndIndex]
	
	printStr += value
	
	return data[valueEndIndex + 1:], printStr
}

func readString(data string, printStr string) (string, string) {
	stringValueIndex := strings.Index(data, ":") + 1
	
	valueLen, _ := strconv.Atoi(data[:stringValueIndex - 1])
	value := data[stringValueIndex:stringValueIndex + valueLen]
	printStr += "\"" + value + "\""
	
	return data[stringValueIndex + valueLen:], printStr
}

func readList(data string, printStr string, tab string) (string, string) {
	data = data[1:] // remove first l
	
	printStr += "[\n"
	tab += "\t"
	for strings.Index(data, "e") != 0 {
		printStr += tab
		data, printStr = Decode(data, printStr, tab)
		printStr += ",\n"
	}
	printStr = printStr[:len(printStr) - 2]
	tab = tab[: len(tab) - 1]
	printStr += "\n" + tab + "]"
	
	return data[1:], printStr
}

func readDict(data string, printStr string, tab string) (string, string) {
	data = data[1:] // remove firs d
	
	printStr += "{\n"
	tab += "\t"
	for strings.Index(data, "e") != 0 {
		printStr += tab
		data, printStr = Decode(data, printStr, tab)
		printStr += " : "
		data, printStr = Decode(data, printStr, tab)
		printStr += ",\n"
	}
	tab = tab[:len(tab) - 1]
	printStr = printStr[:len(printStr) - 2]
	printStr += "\n" + tab + "}"
	
	return data[1:], printStr
}





