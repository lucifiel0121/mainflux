package coap

import "strings"

func authKey(opt interface{}) (string, error) {
	val, ok := opt.(string)
	if !ok {
		return "", errBadOption
	}
	arr := strings.Split(val, "=")
	if len(arr) != 2 || strings.ToLower(arr[0]) != keyHeader {
		return "", errBadOption
	}
	return arr[1], nil
}
