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
