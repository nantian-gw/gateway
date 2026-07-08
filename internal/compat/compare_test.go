package compat

import (
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
)

func TestCompareFilesAllowsAddedField(t *testing.T) {
	prev := testFile(testMessage("ConfigSnapshot",
		testField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))
	curr := testFile(testMessage("ConfigSnapshot",
		testField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
		testField("nonce", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsRemovedField(t *testing.T) {
	prev := testFile(testMessage("ConfigSnapshot",
		testField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
		testField("nonce", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))
	curr := testFile(testMessage("ConfigSnapshot",
		testField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsChangedFieldType(t *testing.T) {
	prev := testFile(testMessage("ConfigSnapshot",
		testField("version", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))
	curr := testFile(testMessage("ConfigSnapshot",
		testField("version", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
	))

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsRemovedMethod(t *testing.T) {
	prev := testFile()
	prev.Service = []*descriptorpb.ServiceDescriptorProto{
		{
			Name: stringPtr("ConfigurationDiscoveryService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       stringPtr("StreamConfiguration"),
				InputType:  stringPtr(".gateway.control.v1.DiscoveryRequest"),
				OutputType: stringPtr(".gateway.control.v1.DiscoveryResponse"),
			}},
		},
	}
	curr := testFile()
	curr.Service = []*descriptorpb.ServiceDescriptorProto{
		{
			Name: stringPtr("ConfigurationDiscoveryService"),
		},
	}

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %#v", result.Errors)
	}
}

func testFile(messages ...*descriptorpb.DescriptorProto) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:        stringPtr("gateway/control/v1/control.proto"),
		Package:     stringPtr("gateway.control.v1"),
		Syntax:      stringPtr("proto3"),
		MessageType: messages,
	}
}

func testMessage(name string, fields ...*descriptorpb.FieldDescriptorProto) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name:  stringPtr(name),
		Field: fields,
	}
}

func testField(name string, number int32, fieldType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	return &descriptorpb.FieldDescriptorProto{
		Name:   stringPtr(name),
		Number: int32Ptr(number),
		Label:  &label,
		Type:   &fieldType,
	}
}

func stringPtr(value string) *string {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func int32ToEnumType(t *testing.T, value int32) *descriptorpb.FieldDescriptorProto_Type {
	v := descriptorpb.FieldDescriptorProto_Type(value)
	return &v
}

func TestCompareFilesWarnsOnFieldRename(t *testing.T) {
	prev := testFile(testMessage("Msg",
		testField("old_name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))
	curr := testFile(testMessage("Msg",
		testField("new_name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for field rename, got %#v", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning for field rename, got %#v", result.Warnings)
	}
}

func TestCompareFilesRejectsRemovedEnum(t *testing.T) {
	prev := testFile()
	prev.EnumType = []*descriptorpb.EnumDescriptorProto{
		{Name: stringPtr("Status")},
	}
	curr := testFile()

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for removed enum, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsRemovedEnumValue(t *testing.T) {
	prev := testFile()
	prev.EnumType = []*descriptorpb.EnumDescriptorProto{
		{Name: stringPtr("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: stringPtr("OK"), Number: int32Ptr(0)},
				{Name: stringPtr("ERROR"), Number: int32Ptr(1)},
			},
		},
	}
	curr := testFile()
	curr.EnumType = []*descriptorpb.EnumDescriptorProto{
		{Name: stringPtr("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: stringPtr("OK"), Number: int32Ptr(0)},
			},
		},
	}

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for removed enum value, got %#v", result.Errors)
	}
}

func TestCompareFilesWarnsOnEnumValueRename(t *testing.T) {
	prev := testFile()
	prev.EnumType = []*descriptorpb.EnumDescriptorProto{
		{Name: stringPtr("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: stringPtr("OLD"), Number: int32Ptr(0)},
			},
		},
	}
	curr := testFile()
	curr.EnumType = []*descriptorpb.EnumDescriptorProto{
		{Name: stringPtr("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: stringPtr("NEW"), Number: int32Ptr(0)},
			},
		},
	}

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for enum value rename, got %#v", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning for enum value rename, got %#v", result.Warnings)
	}
}

func TestCompareFilesRejectsIncompatibleMethodSignature(t *testing.T) {
	prev := testFile()
	prev.Service = []*descriptorpb.ServiceDescriptorProto{
		{Name: stringPtr("Svc"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       stringPtr("Do"),
				InputType:  stringPtr(".v1.Request"),
				OutputType: stringPtr(".v1.Response"),
			}},
		},
	}
	curr := testFile()
	curr.Service = []*descriptorpb.ServiceDescriptorProto{
		{Name: stringPtr("Svc"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       stringPtr("Do"),
				InputType:  stringPtr(".v2.Request"),
				OutputType: stringPtr(".v1.Response"),
			}},
		},
	}

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for incompatible method, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsSyntaxChange(t *testing.T) {
	prev := testFile()
	curr := testFile()
	prev.Syntax = stringPtr("proto2")
	curr.Syntax = stringPtr("proto3")

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for syntax change, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsPackageChange(t *testing.T) {
	prev := testFile()
	curr := testFile()
	prev.Package = stringPtr("old.v1")
	curr.Package = stringPtr("new.v1")

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for package change, got %#v", result.Errors)
	}
}

func TestCompareFilesRejectsMissingMessage(t *testing.T) {
	prev := testFile(testMessage("OldMessage",
		testField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
	))
	curr := testFile()

	result := CompareFiles(prev, curr)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for removed message, got %#v", result.Errors)
	}
}

func TestOKReturnsTrueWhenNoErrors(t *testing.T) {
	r := Result{}
	if !r.OK() {
		t.Fatal("empty result should be OK")
	}
	r.Errors = append(r.Errors, Finding{Path: "x", Message: "bad"})
	if r.OK() {
		t.Fatal("result with errors should not be OK")
	}
	// Warnings alone do not make the result not OK.
	r.Errors = nil
	r.Warnings = append(r.Warnings, Finding{Path: "x", Message: "warning"})
	if !r.OK() {
		t.Fatal("result with only warnings should still be OK")
	}
}

func TestCompareFilesNilPrev(t *testing.T) {
	result := CompareFiles(nil, testFile())
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for nil prev, got %#v", result.Errors)
	}
}

func TestCompareFilesNilCurr(t *testing.T) {
	result := CompareFiles(testFile(), nil)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for nil curr, got %#v", result.Errors)
	}
}
