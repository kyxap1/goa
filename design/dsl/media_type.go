package dsl

import (
	"fmt"
	"mime"
	"strings"

	"bitbucket.org/pkg/inflect"
)
import . "github.com/raphael/goa/design"

// Counter used to create unique media type names for identifier-less media types.
var mediaTypeCount int

// MediaType defines a media type DSL.
//
// MediaType("application/vnd.goa.example.bottle", func() {
//	Description("A bottle of wine")
//	Attributes(func() {
//		Attribute("id", Integer, "ID of bottle")
//		Attribute("href", String, "API href of bottle")
//		Attribute("origin", Origin, "Details on wine origin")
//		Links(func() {
//			Link("origin")
//		})
//              Required("href")
//      })
//	View("default", func() {
//		Attribute("id")
//		Attribute("href")
//		Attribute("links")
//	})
// })
//
// MediaType also refers to a media type (by name or by reference):
//
// 	ResponseTemplate("NotFound", func() {
//		Status(404)
//		MediaType("application/json")
//	})
//
// This function returns the newly defined media type in the first mode, nil otherwise.
func MediaType(val interface{}, dsl ...func()) *MediaTypeDefinition {
	if len(dsl) > 1 {
		ReportError("too many arguments in MediaType call")
		return nil
	}
	if Design == nil {
		InitDesign()
	}
	if Design.MediaTypes == nil {
		Design.MediaTypes = make(map[string]*MediaTypeDefinition)
	}
	if topLevelDefinition(false) {
		identifier, ok := val.(string)
		if !ok {
			ReportError("media type identifier must be a string, got %#v", val)
			return nil
		}
		mediatype, _, err := mime.ParseMediaType(identifier)
		if err != nil {
			ReportError("invalid media type identifier %#v: %s",
				identifier, err)
		}
		elems := strings.Split(mediatype, ".")
		typeName := inflect.Camelize(elems[len(elems)-1])
		if typeName == "" {
			mediaTypeCount++
			typeName = fmt.Sprintf("MediaType%d", mediaTypeCount)
		}
		if _, ok := Design.MediaTypes[identifier]; ok {
			ReportError("media type %#v is defined twice", identifier)
			return nil
		}
		var d func()
		if len(dsl) > 0 {
			d = dsl[0]
		}
		mt := NewMediaTypeDefinition(typeName, identifier, d)
		Design.MediaTypes[identifier] = mt
		return mt
	} else if r, ok := resourceDefinition(false); ok {
		if m, ok := val.(*MediaTypeDefinition); ok {
			if m.UserTypeDefinition == nil {
				ReportError("invalid media type specification, media type is not initialized")
			} else {
				r.MediaType = m.Identifier
				m.Resource = r
			}
		} else if identifier, ok := val.(string); ok {
			r.MediaType = identifier
		} else {
			ReportError("media type must be a string or a *MediaTypeDefinition, got %#v", val)
		}
	} else if r, ok := responseDefinition(true); ok {
		if m, ok := val.(*MediaTypeDefinition); ok {
			r.MediaType = m.Identifier
		} else if identifier, ok := val.(string); ok {
			r.MediaType = identifier
		} else {
			ReportError("media type must be a string or a *MediaTypeDefinition, got %#v", val)
		}
	}
	return nil
}

// BaseType defines the type from which the media or user type is derived from if any. The type can
// be further customized using the Attributes DSL (to add new attributes for example).
//
// Implementation note: BaseType and Attributes may appear in any order in the definition of a type.
// If BaseType appears first then it sets both the BaseType and Type fields of the underlying
// attribute. Running Attributes subsequently then initializes type sub-attributes potentially
// overridding attributes inherited from the base type.
// If Attributes appears first then its sets the type to Object and sets the object attributes.
// Running BaseType subsequently creates a duplicate type and merges the object attributes into
// the copy. The copy is then set as the type of the underlying media or user type attribute.
// TL;DR the implementation of BaseType needs to handle the case where attributes already exist.
func BaseType(t DataType) {
	var att *AttributeDefinition
	if mt, ok := mediaTypeDefinition(false); ok {
		att = mt.AttributeDefinition
		mt.BaseType = t
	} else if ut, ok := typeDefinition(true); ok {
		att = ut.AttributeDefinition
		ut.BaseType = t
	}
	if att != nil {
		dup := t
		if t.IsArray() {
			dup = t.ToArray().Dup()
		} else if t.IsObject() {
			dup = t.ToObject().Dup()
		}
		if att.Type == nil {
			att.Type = dup
		} else if dup.IsObject() && att.Type.IsObject() {
			dup.ToObject().Merge(att.Type.ToObject())
			att.Type = dup
		}
	}
}

// TypeName makes it possible to set the Go struct name in the generated code.
func TypeName(name string) {
	if mt, ok := mediaTypeDefinition(false); ok {
		mt.TypeName = name
	} else if ut, ok := typeDefinition(true); ok {
		ut.TypeName = name
	}
}

// View adds a new view to the media type.
// It takes the view name and the DSL defining it.
// View can also be used to specify the view used to render an attribute.
func View(name string, dsl ...func()) {
	if mt, ok := mediaTypeDefinition(false); ok {
		if !mt.Type.IsObject() && !mt.Type.IsArray() {
			ReportError("cannot define view on non object and non collection media types")
			return
		}
		if mt.Views == nil {
			mt.Views = make(map[string]*ViewDefinition)
		} else {
			if _, ok = mt.Views[name]; ok {
				ReportError("multiple definitions for view %#v in media type %#v", name, mt.TypeName)
				return
			}
		}
		at := &AttributeDefinition{}
		ok := false
		if len(dsl) > 0 {
			ok = executeDSL(dsl[0], at)
		} else if mt.Type.IsArray() {
			// inherit view from collection element if present
			elem := mt.Type.ToArray().ElemType
			if elem != nil {
				if pa, ok2 := elem.Type.(*MediaTypeDefinition); ok2 {
					if v, ok2 := pa.Views[name]; ok2 {
						at = v.AttributeDefinition
						ok = true
					} else {
						ReportError("unknown view %#v", name)
						return
					}
				}
			}
		}
		if ok {
			o := at.Type.ToObject()
			if o != nil {
				mto := mt.Type.ToObject()
				if mto == nil {
					mto = mt.Type.ToArray().ElemType.Type.ToObject()
				}
				for n := range o {
					if existing, ok := mto[n]; ok {
						o[n] = existing
					} else if n != "links" {
						ReportError("unknown attribute %#v", n)
					}
				}
			}
			mt.Views[name] = &ViewDefinition{
				AttributeDefinition: at,
				Name:                name,
				Parent:              mt,
			}
		}
	} else if a, ok := attributeDefinition(true); ok {
		a.View = name
	}
}

// Attributes defines the media type attributes DSL.
func Attributes(dsl func()) {
	if mt, ok := mediaTypeDefinition(true); ok {
		executeDSL(dsl, mt)
	}
}

// Links defines the media type links DSL.
func Links(dsl func()) {
	if mt, ok := mediaTypeDefinition(true); ok {
		executeDSL(dsl, mt)
	}
}

// Link defines a media type link DSL.
// At the minimum a link has a name corresponding to one of the media type attribute names.
// A link may also define the view used to render the link content if different
// from "link".
// Examples:
//
// Link("vendor")
//
// Link("vendor", "view")
//
func Link(name string, view ...string) {
	if mt, ok := mediaTypeDefinition(true); ok {
		if mt.Links == nil {
			mt.Links = make(map[string]*LinkDefinition)
		} else {
			if _, ok := mt.Links[name]; ok {
				ReportError("duplicate definition for link %#v", name)
				return
			}
		}
		link := &LinkDefinition{Name: name, Parent: mt}
		if len(view) > 1 {
			ReportError("invalid syntax in Link definition for %#v, allowed syntax is Link(name) or Link(name, view)", name)
		}
		if len(view) > 0 {
			link.View = view[0]
		} else {
			link.View = "link"
		}
		mt.Links[name] = link
	}
}

// ArrayOf creates an array from its element type.
func ArrayOf(t DataType) *Array {
	at := AttributeDefinition{Type: t}
	return &Array{ElemType: &at}
}

// CollectionOf creates a collection media type from its element media type.
// A collection media type represents the content of responses that return a
// collection of resources such as "index" actions.
// TBD: this relies on the underlying media type to have been evaled already.
func CollectionOf(m *MediaTypeDefinition, dsl ...func()) *MediaTypeDefinition {
	id := m.Identifier
	mediatype, params, err := mime.ParseMediaType(id)
	if err != nil {
		ReportError("invalid media type identifier %#v: %s", id, err)
		return nil
	}
	hasType := false
	for param := range params {
		if param == "type" {
			hasType = true
			break
		}
	}
	if !hasType {
		params["type"] = "collection"
		id = mime.FormatMediaType(mediatype, params)
	}
	typeName := m.TypeName + "Collection"
	mt := NewMediaTypeDefinition(typeName, id, func() {
		if mt, ok := mediaTypeDefinition(true); ok {
			mt.TypeName = typeName
			mt.AttributeDefinition = &AttributeDefinition{Type: ArrayOf(m)}
			if len(dsl) > 0 {
				executeDSL(dsl[0], mt)
			}
		}
	})
	if executeDSL(mt.DSL, mt) {
		Design.MediaTypes[id] = mt
	}
	return mt
}
