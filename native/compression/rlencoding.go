package compression

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func RLEncode(data string) string {
	var result string
	count := 1
	for i := 0; i < len(data); i++ {
		if i+1 < len(data) && data[i] == data[i+1] {
			count++
			continue
		}
		result += fmt.Sprintf("%d%c", count, data[i])
		count = 1
	}
	return result
}

func RLDecode(encoded string) string {
	var result string
	regex := regexp.MustCompile(`(\d+)(\w)`)

	for _, match := range regex.FindAllStringSubmatch(encoded, -1) {
		num, _ := strconv.Atoi(match[1])
		result += strings.Repeat(match[2], num)
	}

	return result
}

func RLEdecodebytes(data []byte) []byte {
	var result []byte

	for i := 0; i < len(data); i += 2 {
		count := int(data[i])
		result = append(result, bytes.Repeat([]byte{data[i+1]}, count)...)
	}

	return result
}
