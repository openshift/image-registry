package wrapped

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/docker/distribution/context"
)

func zeroIn(typ reflect.Type) []reflect.Value {
	var args []reflect.Value
	for i := 0; i < typ.NumIn(); i++ {
		if i == typ.NumIn()-1 && typ.IsVariadic() {
			break
		}
		arg := reflect.Zero(typ.In(i))
		args = append(args, arg)
	}
	return args
}

func TestWrapper(t *testing.T) {
	var lastFuncName string
	captureFuncName := func(ctx context.Context, funcname string, f func(ctx context.Context) error) error {
		lastFuncName = funcname
		return fmt.Errorf("don't call upstream code")
	}

	exclude := map[string]bool{
		"BlobWriter.Close":     true,
		"BlobWriter.ID":        true,
		"BlobWriter.ReadFrom":  true,
		"BlobWriter.Size":      true,
		"BlobWriter.StartedAt": true,
		"BlobWriter.Write":     true,
	}

	wrappers := []reflect.Value{
		reflect.ValueOf(&blobStore{wrapper: captureFuncName}),
		reflect.ValueOf(&blobWriter{wrapper: captureFuncName}),
		reflect.ValueOf(&manifestService{wrapper: captureFuncName}),
		reflect.ValueOf(&tagService{wrapper: captureFuncName}),
	}
	for _, v := range wrappers {
		typeName := strings.Title(v.Elem().Type().Name())
		for i := 0; i < v.Type().NumMethod(); i++ {
			lastFuncName = "unhandled"

			methodName := v.Type().Method(i).Name
			funcName := fmt.Sprintf("%s.%s", typeName, methodName)

			method := v.Method(i)
			args := zeroIn(method.Type())
			func() {
				defer func() {
					// BlobWriter.Close and other unhandled methods may panic
					recover()
				}()
				method.Call(args)
			}()

			expectedFuncName := funcName
			if exclude[expectedFuncName] {
				expectedFuncName = "unhandled"
			}

			if lastFuncName != expectedFuncName {
				t.Errorf("%s: got %q, want %q", funcName, lastFuncName, expectedFuncName)
			}
		}
	}
}
