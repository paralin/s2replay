package s2replay

import (
	"strconv"
	"strings"
)

var itemCounts = map[string]int{
	"MAX_ITEM_STOCKS":             8,
	"MAX_ABILITY_DRAFT_ABILITIES": 48,
}

var implicitPointerTypes = map[string]bool{
	"CBodyComponent":    true,
	"CLightComponent":   true,
	"CPhysicsComponent": true,
	"CRenderComponent":  true,
	"CPlayerLocalData":  true,
}

type fieldType struct {
	baseType    string
	genericType *fieldType
	pointer     bool
	count       int
}

func newFieldType(name string) *fieldType {
	name = strings.TrimSpace(name)
	t := &fieldType{}
	baseEnd := len(name)
	if i := indexByte(name, '<'); i >= 0 {
		baseEnd = i
		close := lastIndexByte(name, '>')
		if close > i+1 {
			t.genericType = newFieldType(name[i+1 : close])
		}
	}
	if i := indexByte(name, '*'); i >= 0 && i < baseEnd {
		baseEnd = i
		t.pointer = true
	}
	if i := indexByte(name, '['); i >= 0 {
		if i < baseEnd {
			baseEnd = i
		}
		if close := lastIndexByte(name, ']'); close > i+1 {
			count := name[i+1 : close]
			if n, ok := itemCounts[count]; ok {
				t.count = n
			} else if n, err := strconv.Atoi(count); err == nil && n > 0 {
				t.count = n
			} else {
				t.count = 1024
			}
		}
	}
	t.baseType = name[:baseEnd]
	if implicitPointerTypes[t.baseType] {
		t.pointer = true
	}
	if t.baseType == "char" && t.count == 0 {
		t.baseType = "uint8"
	}
	return t
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
