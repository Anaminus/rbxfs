package rbxfs

import (
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxfile"
	"github.com/robloxapi/rbxfile/bin"
	"io"
	"strings"
)

func GetFormatFromExt(ext string) Format {
	ext = strings.TrimPrefix(ext, ".")
	switch ext {
	case FormatRBXM{}.Ext():
		return &FormatRBXM{}
	case FormatRBXMX{}.Ext():
		return &FormatRBXMX{}
	case FormatRBXL{}.Ext():
		return &FormatRBXL{}
	case FormatRBXLX{}.Ext():
		return &FormatRBXLX{}
	case FormatJSON{}.Ext():
		return &FormatJSON{}
	case FormatXML{}.Ext():
		return &FormatXML{}
	case FormatBin{}.Ext():
		return &FormatBin{}
	case FormatLua{}.Ext():
		return &FormatLua{}
	case FormatText{}.Ext():
		return &FormatText{}
	}
	return nil
}

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
	Ext() string
	API() *rbxapi.API
	SetAPI(api *rbxapi.API)
	// CanEncode returns whether the selections can be encoded.
	CanEncode(selections []OutSelection) bool
	// Encode encodes the selection in a format written to w.
	Encode(w io.Writer, selections []OutSelection) error
	// Decode reads formatted data from r, and decodes it into an ItemSource.
	Decode(r io.Reader) (*ItemSource, error)
}

type FormatRBXM struct {
	api *rbxapi.API
}

func (FormatRBXM) Name() string {
	return "RBXM"
}
func (FormatRBXM) Ext() string {
	return "rbxm"
}
func (f FormatRBXM) API() *rbxapi.API {
	return f.api
}
func (f *FormatRBXM) SetAPI(api *rbxapi.API) {
	f.api = api
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
	if err := bin.SerializeModel(w, f.api, &rbxfile.Root{Instances: instances}); err != nil {
		//ERROR:
		return ErrFmtEncode{err}
	}
	return nil
}
func (f FormatRBXM) Decode(r io.Reader) (is *ItemSource, err error) {
	root, err := bin.DeserializeModel(r, f.api)
	if err != nil {
		err = ErrFmtDecode{err}
		return
	}
	return
}

type FormatRBXMX struct {
	api *rbxapi.API
}

func (FormatRBXMX) Name() string {
	return "RBXMX"
}
func (FormatRBXMX) Ext() string {
	return "rbxmx"
}
func (f FormatRBXMX) API() *rbxapi.API {
	return f.api
}
func (f *FormatRBXMX) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (FormatRBXMX) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatRBXL struct {
	api *rbxapi.API
}

func (FormatRBXL) Name() string {
	return "RBXL"
}
func (FormatRBXL) Ext() string {
	return "rbxl"
}
func (f FormatRBXL) API() *rbxapi.API {
	return f.api
}
func (f *FormatRBXL) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (f FormatRBXL) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatRBXLX struct {
	api *rbxapi.API
}

func (FormatRBXLX) Name() string {
	return "RBXLX"
}
func (FormatRBXLX) Ext() string {
	return "rbxlx"
}
func (f FormatRBXLX) API() *rbxapi.API {
	return f.api
}
func (f *FormatRBXLX) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (FormatRBXLX) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatJSON struct {
	api *rbxapi.API
}

func (FormatJSON) Name() string {
	return "JSON"
}
func (FormatJSON) Ext() string {
	return "json"
}
func (f FormatJSON) API() *rbxapi.API {
	return f.api
}
func (f *FormatJSON) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (FormatJSON) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatXML struct {
	api *rbxapi.API
}

func (FormatXML) Name() string {
	return "XML"
}
func (FormatXML) Ext() string {
	return "xml"
}
func (f FormatXML) API() *rbxapi.API {
	return f.api
}
func (f *FormatXML) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (FormatXML) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatBin struct {
	api *rbxapi.API
}

func (FormatBin) Name() string {
	return "Bin"
}
func (FormatBin) Ext() string {
	return "bin"
}
func (f FormatBin) API() *rbxapi.API {
	return f.api
}
func (f *FormatBin) SetAPI(api *rbxapi.API) {
	f.api = api
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
func (FormatBin) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatLua struct {
	api *rbxapi.API
}

func (FormatLua) Name() string {
	return "Lua"
}
func (FormatLua) Ext() string {
	return "lua"
}
func (f FormatLua) API() *rbxapi.API {
	return f.api
}
func (f *FormatLua) SetAPI(api *rbxapi.API) {
	f.api = api
}
func (FormatLua) CanEncode(sel []OutSelection) bool {
	return len(sel) == 1 &&
		len(sel[0].Children) == 0 &&
		len(sel[0].Properties) == 1
}
func (FormatLua) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatLua) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}

type FormatText struct {
	api *rbxapi.API
}

func (FormatText) Name() string {
	return "Text"
}
func (FormatText) Ext() string {
	return "txt"
}
func (f FormatText) API() *rbxapi.API {
	return f.api
}
func (f *FormatText) SetAPI(api *rbxapi.API) {
	f.api = api
}
func (FormatText) CanEncode(sel []OutSelection) bool {
	return len(sel) == 1 &&
		len(sel[0].Children) == 0 &&
		len(sel[0].Properties) == 1
}
func (FormatText) Encode(w io.Writer, sel []OutSelection) error {
	return nil
}
func (FormatText) Decode(r io.Reader) (is *ItemSource, err error) {
	return
}
