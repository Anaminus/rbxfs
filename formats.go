package rbxfs

import (
	"fmt"
	"github.com/robloxapi/rbxdump"
	"github.com/robloxapi/rbxfile"
	"github.com/robloxapi/rbxfile/bin"
	"io"
)

type ErrFmtSelection struct {
	Format    string
	Selection OutSelection
}

func (err ErrFmtSelection) Error() string {
	return fmt.Sprintf("selection not supported by %s format")
}

type ErrFmtBounds struct {
	Format string
	Type   string
	Index  int
	Value  int
	Max    int
}

func (err ErrFmtBounds) Error() string {
	if err.Value < 0 {
		return fmt.Sprintf("%s selection %d out of bounds (%d < 0)", err.Type, err.Index, err.Value)
	} else {
		return fmt.Sprintf("%s selection %d out of bounds (%d >= %d)", err.Type, err.Index, err.Value, err.Max)
	}
}

type ErrFmtEncode struct {
	Err error
}

func (err ErrFmtEncode) Error() string {
	return fmt.Sprintf("failed to encode format: %s", err.Err.Error())
}

type ErrFmtDecode struct {
	Err error
}

func (err ErrFmtDecode) Error() string {
	return fmt.Sprintf("failed to decode format: %s", err.Err.Error())
}

type Format interface {
	Name() string
	// Check receives amounts of objects, properties and values, and returns
	// whether the format will be able to encode them.
	Check(obj, prop, val int) bool
	// Encode uses Selection sel to read data from obj, and encode it to a format written to w.
	Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error
	// Decode reads formatted data from r, and decodes it into objects,
	// properties, and values, added to obj.
	Decode(r io.Reader) (ItemSource, error)
}

type FormatRBXM struct {
	API *rbxdump.API
}

func (FormatRBXM) Name() string {
	return "RBXM"
}
func (FormatRBXM) Check(obj, prop, val int) bool {
	return obj >= 0 && prop == 0 && val == 0
}
func (f FormatRBXM) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	if !f.Check(len(sel.Children), len(sel.Properties), len(sel.Values)) {
		return ErrFmtSelection{f.Name(), sel}
	}
	instances := make([]*rbxfile.Instance, len(sel.Children))
	for i, v := range sel.Children {
		if v < 0 || v >= len(obj.Children) {
			return ErrFmtBounds{f.Name(), "child", i, v, len(obj.Children)}
		}
		instances[i] = obj.Children[v]
	}
	if err := bin.SerializeModel(w, f.API, &rbxfile.Root{Instances: instances}); err != nil {
		return ErrFmtEncode{err}
	}
	return nil
}
func (f FormatRBXM) Decode(r io.Reader) (is ItemSource, err error) {
	_, err = bin.DeserializeModel(r, f.API)
	if err != nil {
		err = ErrFmtDecode{err}
		return
	}
	return
}

type FormatRBXMX struct {
	API *rbxdump.API
}

func (FormatRBXMX) Name() string {
	return "RBXMX"
}
func (FormatRBXMX) Check(obj, prop, val int) bool {
	return obj >= 0 && prop == 0 && val == 0
}
func (FormatRBXMX) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatRBXMX) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatJSON struct{}

func (FormatJSON) Name() string {
	return "JSON"
}
func (FormatJSON) Check(obj, prop, val int) bool {
	return obj == 0 && prop >= 0 && val == 0
}
func (FormatJSON) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatJSON) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatXML struct{}

func (FormatXML) Name() string {
	return "XML"
}
func (FormatXML) Check(obj, prop, val int) bool {
	return obj == 0 && prop >= 0 && val == 0
}
func (FormatXML) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatXML) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatBin struct{}

func (FormatBin) Name() string {
	return "Bin"
}
func (FormatBin) Check(obj, prop, val int) bool {
	return obj == 0 && prop == 0 && val == 1
}
func (FormatBin) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatBin) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatLua struct{}

func (FormatLua) Name() string {
	return "Lua"
}
func (FormatLua) Check(obj, prop, val int) bool {
	return obj == 0 && prop == 0 && val == 1
}
func (FormatLua) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatLua) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatText struct{}

func (FormatText) Name() string {
	return "Text"
}
func (FormatText) Check(obj, prop, val int) bool {
	return obj == 0 && prop == 0 && val == 1
}
func (FormatText) Encode(w io.Writer, obj *rbxfile.Instance, sel OutSelection) error {
	return nil
}
func (FormatText) Decode(r io.Reader) (is ItemSource, err error) {
	return
}
