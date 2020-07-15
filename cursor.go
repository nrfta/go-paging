package paging

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// EncodeOffsetCursor takes an integer and encodes to a base64 string as "cursor:offset:NUMBER"
func EncodeOffsetCursor(offset int) *string {
	data := "cursor:offset:" + strconv.Itoa(offset)
	encoded := base64.URLEncoding.EncodeToString([]byte(data))
	return &encoded
}

// DecodeOffsetCursor takes a base64 string and decotes it to extract the
// offset from a string based on "cursor:offset:NUMBER". It defails to 0 if cannot decode or has any error.
func DecodeOffsetCursor(input *string) int {
	if input == nil {
		return 0
	}

	var decoded []byte
	var data []string
	var err error

	if decoded, err = base64.URLEncoding.DecodeString(*input); err != nil {
		return 0
	}

	if data = strings.Split(string(decoded), ":"); len(data) == 3 {
		offset, err := strconv.ParseInt(data[2], 10, 32)

		if err != nil {
			return 0
		}
		return int(offset)
	}

	return 0
}
