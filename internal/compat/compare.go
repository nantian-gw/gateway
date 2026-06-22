package compat

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/types/descriptorpb"
)

type Finding struct {
	Path    string
	Message string
}

type Result struct {
	Errors   []Finding
	Warnings []Finding
}

func (r Result) OK() bool {
	return len(r.Errors) == 0
}

func CompareFiles(prev, curr *descriptorpb.FileDescriptorProto) Result {
	var result Result

	if prev == nil {
		result.addError("file", "previous descriptor is missing")
		return result
	}
	if curr == nil {
		result.addError("file", "current descriptor is missing")
		return result
	}

	if prev.GetSyntax() != curr.GetSyntax() {
		result.addError(
			"file",
			"syntax changed from %q to %q",
			prev.GetSyntax(),
			curr.GetSyntax(),
		)
	}
	if prev.GetPackage() != curr.GetPackage() {
		result.addError(
			"file",
			"package changed from %q to %q",
			prev.GetPackage(),
			curr.GetPackage(),
		)
	}

	compareEnumLists(&result, "file", prev.GetEnumType(), curr.GetEnumType())
	compareMessageLists(&result, "file", prev.GetMessageType(), curr.GetMessageType())
	compareServiceLists(&result, "file", prev.GetService(), curr.GetService())

	return result
}

func compareMessageLists(result *Result, scope string, prev, curr []*descriptorpb.DescriptorProto) {
	currByName := make(map[string]*descriptorpb.DescriptorProto, len(curr))
	for _, item := range curr {
		if item.GetOptions().GetMapEntry() {
			continue
		}
		currByName[item.GetName()] = item
	}

	prevNames := make([]string, 0, len(prev))
	for _, item := range prev {
		if item.GetOptions().GetMapEntry() {
			continue
		}
		prevNames = append(prevNames, item.GetName())
	}
	sort.Strings(prevNames)

	for _, name := range prevNames {
		prevItem := findMessageByName(prev, name)
		currItem := currByName[name]
		path := fmt.Sprintf("%s message %s", scope, name)
		if currItem == nil {
			result.addError(path, "message was removed")
			continue
		}
		compareMessage(result, path, prevItem, currItem)
	}
}

func compareMessage(result *Result, scope string, prev, curr *descriptorpb.DescriptorProto) {
	prevFields := make(map[int32]*descriptorpb.FieldDescriptorProto, len(prev.GetField()))
	currFields := make(map[int32]*descriptorpb.FieldDescriptorProto, len(curr.GetField()))

	for _, field := range prev.GetField() {
		prevFields[field.GetNumber()] = field
	}
	for _, field := range curr.GetField() {
		currFields[field.GetNumber()] = field
	}

	fieldNumbers := make([]int, 0, len(prevFields))
	for number := range prevFields {
		fieldNumbers = append(fieldNumbers, int(number))
	}
	sort.Ints(fieldNumbers)

	for _, rawNumber := range fieldNumbers {
		number := int32(rawNumber)
		prevField := prevFields[number]
		currField := currFields[number]
		path := fmt.Sprintf("%s field %d", scope, number)
		if currField == nil {
			result.addError(path, "field %q was removed", prevField.GetName())
			continue
		}

		prevSig := fieldSignature(prevField)
		currSig := fieldSignature(currField)
		if prevSig != currSig {
			result.addError(
				path,
				"incompatible signature change for %q: previous=%s current=%s",
				prevField.GetName(),
				prevSig,
				currSig,
			)
			continue
		}

		if prevField.GetName() != currField.GetName() {
			result.addWarning(
				path,
				"field name changed from %q to %q while keeping the same number",
				prevField.GetName(),
				currField.GetName(),
			)
		}
	}

	compareEnumLists(result, scope, prev.GetEnumType(), curr.GetEnumType())
	compareMessageLists(result, scope, prev.GetNestedType(), curr.GetNestedType())
}

func compareEnumLists(result *Result, scope string, prev, curr []*descriptorpb.EnumDescriptorProto) {
	currByName := make(map[string]*descriptorpb.EnumDescriptorProto, len(curr))
	for _, item := range curr {
		currByName[item.GetName()] = item
	}

	prevNames := make([]string, 0, len(prev))
	for _, item := range prev {
		prevNames = append(prevNames, item.GetName())
	}
	sort.Strings(prevNames)

	for _, name := range prevNames {
		prevItem := findEnumByName(prev, name)
		currItem := currByName[name]
		path := fmt.Sprintf("%s enum %s", scope, name)
		if currItem == nil {
			result.addError(path, "enum was removed")
			continue
		}
		compareEnum(result, path, prevItem, currItem)
	}
}

func compareEnum(result *Result, scope string, prev, curr *descriptorpb.EnumDescriptorProto) {
	prevValues := make(map[int32]*descriptorpb.EnumValueDescriptorProto, len(prev.GetValue()))
	currValues := make(map[int32]*descriptorpb.EnumValueDescriptorProto, len(curr.GetValue()))

	for _, value := range prev.GetValue() {
		prevValues[value.GetNumber()] = value
	}
	for _, value := range curr.GetValue() {
		currValues[value.GetNumber()] = value
	}

	valueNumbers := make([]int, 0, len(prevValues))
	for number := range prevValues {
		valueNumbers = append(valueNumbers, int(number))
	}
	sort.Ints(valueNumbers)

	for _, rawNumber := range valueNumbers {
		number := int32(rawNumber)
		prevValue := prevValues[number]
		currValue := currValues[number]
		path := fmt.Sprintf("%s value %d", scope, number)
		if currValue == nil {
			result.addError(path, "enum value %q was removed", prevValue.GetName())
			continue
		}
		if prevValue.GetName() != currValue.GetName() {
			result.addWarning(
				path,
				"enum value name changed from %q to %q while keeping the same number",
				prevValue.GetName(),
				currValue.GetName(),
			)
		}
	}
}

func compareServiceLists(result *Result, scope string, prev, curr []*descriptorpb.ServiceDescriptorProto) {
	currByName := make(map[string]*descriptorpb.ServiceDescriptorProto, len(curr))
	for _, item := range curr {
		currByName[item.GetName()] = item
	}

	prevNames := make([]string, 0, len(prev))
	for _, item := range prev {
		prevNames = append(prevNames, item.GetName())
	}
	sort.Strings(prevNames)

	for _, name := range prevNames {
		prevItem := findServiceByName(prev, name)
		currItem := currByName[name]
		path := fmt.Sprintf("%s service %s", scope, name)
		if currItem == nil {
			result.addError(path, "service was removed")
			continue
		}
		compareService(result, path, prevItem, currItem)
	}
}

func compareService(result *Result, scope string, prev, curr *descriptorpb.ServiceDescriptorProto) {
	currMethods := make(map[string]*descriptorpb.MethodDescriptorProto, len(curr.GetMethod()))
	for _, method := range curr.GetMethod() {
		currMethods[method.GetName()] = method
	}

	prevNames := make([]string, 0, len(prev.GetMethod()))
	for _, method := range prev.GetMethod() {
		prevNames = append(prevNames, method.GetName())
	}
	sort.Strings(prevNames)

	for _, name := range prevNames {
		prevMethod := findMethodByName(prev.GetMethod(), name)
		currMethod := currMethods[name]
		path := fmt.Sprintf("%s method %s", scope, name)
		if currMethod == nil {
			result.addError(path, "method was removed")
			continue
		}

		prevSig := methodSignature(prevMethod)
		currSig := methodSignature(currMethod)
		if prevSig != currSig {
			result.addError(
				path,
				"incompatible method signature change: previous=%s current=%s",
				prevSig,
				currSig,
			)
		}
	}
}

func fieldSignature(field *descriptorpb.FieldDescriptorProto) string {
	return fmt.Sprintf(
		"label=%s type=%s type_name=%q oneof=%d proto3_optional=%t",
		field.GetLabel().String(),
		field.GetType().String(),
		field.GetTypeName(),
		field.GetOneofIndex(),
		field.GetProto3Optional(),
	)
}

func methodSignature(method *descriptorpb.MethodDescriptorProto) string {
	return fmt.Sprintf(
		"input=%q output=%q client_streaming=%t server_streaming=%t",
		method.GetInputType(),
		method.GetOutputType(),
		method.GetClientStreaming(),
		method.GetServerStreaming(),
	)
}

func findMessageByName(items []*descriptorpb.DescriptorProto, name string) *descriptorpb.DescriptorProto {
	for _, item := range items {
		if item.GetName() == name {
			return item
		}
	}
	return nil
}

func findEnumByName(items []*descriptorpb.EnumDescriptorProto, name string) *descriptorpb.EnumDescriptorProto {
	for _, item := range items {
		if item.GetName() == name {
			return item
		}
	}
	return nil
}

func findServiceByName(items []*descriptorpb.ServiceDescriptorProto, name string) *descriptorpb.ServiceDescriptorProto {
	for _, item := range items {
		if item.GetName() == name {
			return item
		}
	}
	return nil
}

func findMethodByName(items []*descriptorpb.MethodDescriptorProto, name string) *descriptorpb.MethodDescriptorProto {
	for _, item := range items {
		if item.GetName() == name {
			return item
		}
	}
	return nil
}

func (r *Result) addError(path, format string, args ...any) {
	r.Errors = append(r.Errors, Finding{
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	})
}

func (r *Result) addWarning(path, format string, args ...any) {
	r.Warnings = append(r.Warnings, Finding{
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	})
}
