package rbxfs

import (
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxfile"
	"github.com/robloxapi/rbxfile/bin"
	"io"
)

type ErrFmtSelection struct {
	Format string
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
	// CanEncode returns whether the selections can be encoded.
	CanEncode(selections []OutSelection) bool
	// Encode uses Selection sel to read data from obj, and encode it to a format written to w.
	Encode(w io.Writer, selections []OutSelection) error
	// Decode reads formatted data from r, and decodes it into objects,
	// properties, and values, added to obj.
	Decode(r io.Reader) (ItemSource, error)
}

type FormatRBXM struct {
	API *rbxapi.API
}

func (FormatRBXM) Name() string {
	return "RBXM"
}
func (FormatRBXM) CanEncode(sel []OutSelection) bool {
	for _, s := range sel {
		if len(s.Properties) > 0 {
			return false
		}
	}
	return true
}
func (f FormatRBXM) Encode(w io.Writer, selections []OutSelection) error {
	if !f.CanEncode(selections) {
		//ERROR:
		return ErrFmtSelection{f.Name()}
	}

	n := 0
	for _, s := range selections {
		n += len(s.Children)
	}

	instances := make([]*rbxfile.Instance, 0, n)
	for _, s := range selections {
		for i, v := range s.Children {
			if v < 0 || v >= len(s.Object.Children) {
				//ERROR:
				return ErrFmtBounds{f.Name(), "child", i, v, len(s.Object.Children)}
			}
			instances = append(instances, s.Object.Children[v])
		}
	}
	if err := bin.SerializeModel(w, f.API, &rbxfile.Root{Instances: instances}); err != nil {
		//ERROR:
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
	API *rbxapi.API
}

func (FormatRBXMX) Name() string {
	return "RBXMX"
}
func (FormatRBXMX) CanEncode(sel []OutSelection) bool {
	for _, s := range sel {
		if len(s.Properties) > 0 {
			return false
		}
	}
	return true
}
func (FormatRBXMX) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatRBXMX) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatRBXL struct {
	API *rbxapi.API
}

func (FormatRBXL) Name() string {
	return "RBXL"
}
func (FormatRBXL) CanEncode(sel []OutSelection) bool {
	for _, s := range sel {
		if len(s.Properties) > 0 {
			return false
		}
	}
	return true
}
func (f FormatRBXL) Encode(w io.Writer, selections []OutSelection) error {
	return nil
}
func (f FormatRBXL) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatRBXLX struct {
	API *rbxapi.API
}

func (FormatRBXLX) Name() string {
	return "RBXLX"
}
func (FormatRBXLX) CanEncode(sel []OutSelection) bool {
	for _, s := range sel {
		if len(s.Properties) > 0 {
			return false
		}
	}
	return true
}
func (FormatRBXLX) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatRBXLX) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatJSON struct{}

func (FormatJSON) Name() string {
	return "JSON"
}
func (FormatJSON) CanEncode(sel []OutSelection) bool {
	if len(sel) > 1 {
		return false
	} else if len(sel) == 1 && len(sel[0].Children) > 0 {
		return false
	}
	return true
}
func (FormatJSON) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatJSON) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatXML struct{}

func (FormatXML) Name() string {
	return "XML"
}
func (FormatXML) CanEncode(sel []OutSelection) bool {
	if len(sel) > 1 {
		return false
	} else if len(sel) == 1 && len(sel[0].Children) > 0 {
		return false
	}
	return true
}
func (FormatXML) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatXML) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatBin struct{}

func (FormatBin) Name() string {
	return "Bin"
}
func (FormatBin) CanEncode(sel []OutSelection) bool {
	return len(sel) == 1 &&
		len(sel[0].Children) == 0 &&
		len(sel[0].Properties) == 1
}
func (FormatBin) Check(obj, prop, val int) bool {
	return obj == 0 && prop == 0 && val == 1
}
func (FormatBin) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatBin) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatLua struct{}

func (FormatLua) Name() string {
	return "Lua"
}
func (FormatLua) CanEncode(sel []OutSelection) bool {
	return len(sel) == 1 &&
		len(sel[0].Children) == 0 &&
		len(sel[0].Properties) == 1
}
func (FormatLua) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatLua) Decode(r io.Reader) (is ItemSource, err error) {
	return
}

type FormatText struct{}

func (FormatText) Name() string {
	return "Text"
}
func (FormatText) CanEncode(sel []OutSelection) bool {
	return len(sel) == 1 &&
		len(sel[0].Children) == 0 &&
		len(sel[0].Properties) == 1
}
func (FormatText) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatText) Decode(r io.Reader) (is ItemSource, err error) {
	return
}
