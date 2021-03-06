# RBXFS Rules

The behavior of rbxfs is determined by "rules".

Rules cascade. If we are traversing a tree, then rules are applied according
to the tree. As we move deeper into the tree, rules in the current node are
merged with rules from above.

When syncing, consider a list of rules. Rules that appear later in the list
take precedence over rules earlier in the list.

To begin, this list is filled with the global rules, which apply over all
projects. Next, this list is merged with the project rules, which apply for
the current project.

Each directory, representing an object, is then traversed. Each directory can
have its own list of rules that are merged into the current list. These are
contained within a file called `.rbxfsrules`.

## Rule File Syntax

*I want the syntax to be kept simple. If this leads to more verbosity when
defining rules, then so be it.*

In general, whitespace is used to separate tokens. Lines beginning with `#`
are comments.

Each rule has the following general syntax:

```
<type> <pattern> `:` <filter> `\n`
```

`type` is one of the following:
- `out`: a rule applied when reading data out of a place.
- `in`: a rule applied when reading data into a place.

The syntax of both patterns and filters are the following:

```
<name> `(` [ <argument> { `,` <argument> } ] `)`
```

That is, both patterns and filters appear similar to a function call in many
programming languages.

### Argument types

A type is used to describe the syntax of an argument for a defined pattern or
filter.

In general, an argument may be a sequence of any characters, terminated by `,`
(next argument) or `)` (end of arguments). `\` May be used to escape the next
character. This can be used to treat `,` or `)` characters literally, as well
as any character interpreted specially by the argument type. Note that leading
and trailing whitespace characters are not ignored.

Types:

- String
	- A string value. Basically any text as described above.
- Name
	- A string used to match an item.
	- `*`: Match anything.
	- (text): Match the literal text.
- Class
	- Similar to Name. Indicates the name of a class.
	- Can be prefixed with `@` to select only the class name, and not any
	  classes that inherit from it.
- FileName
	- A file name.
	- Any characters that make up a valid file name (enforced by the
	  filesystem rather than the parser).
	- May contain `*`, which matches 0 or more characters.

### Patterns and Filters

There are two predefined sets of patterns and filters, one for `out` rules,
and another for `in` rules.

#### Out Patterns

- `Child(class Class)`
	- Select children of the current object.
	- `class`: The class name of the object.
- `Property(class Class, property Name, Type Name = *)`
	- Select properties of the current object.
	- `class`: Matches the property if the value matches the class of the
	  current object.
	- `property`: Matches the name of the property.
	- `type`: Matches the name of the property's type.

#### Out Filters

- `File(name String)`
	- Write selected items to a file in the current directory.
	- `name` determines the file name. The format of the file is determined by the extension.
	- The following formats are supported for objects:
		- `rbxm`: Binary Roblox Model
		- `rbxmx`: XML Roblox Model
	- The following formats are supported for properties:
		- `json`
		- `xml`
	- Any number of items can be matched to the same file, though an item will be written once, at most.
- `Directory()`
	- Write selected objects as directories.
	- The name of each directory is the Name property of each object.
	- If the Name property is not valid as a directory name, then the object is not matched.
- `PropertyName(format String)`
	- Writes selected properties to named files.
	- The name of a selected property determines the base name of the file.
	- `format`: Determines the format and extension of the file.
	- The following formats are supported:
		- `bin`: Receives a BinaryString, and writes the value in raw binary format.
		- `lua`: Receives a ProtectedString and writes the value encoded in UTF-8.
		- `txt`: Receives a String and writes the value encoded in UTF-8.
- `Ignore()`
	- Ignore selected objects.

#### In Patterns

- `File(name FileName)`
	- Select a file (not directory) by name.
- `Directory(class Class, name FileName)`
	- Select a directory.
	- First selects by the class name associated with the directory (stored
      under `(directory)/ClassName`).
    - Then selects by the name of the directory.

#### In Filters

- `Children()`
	- Map the contents of selected files to the children of the current object.
	- Files must be in formats supported by out.filter.File.
- `Properties()`
	- Map the contents of selected files to the properties of the current object.
	- Files must in formats supported by out.filter.Directory.
- `Property(prop String)`
	- Map the contents of a selected file to the value of a given property.
	- Cannot match more than one file.
	- `prop`: The name of the property to be mapped to.
	- If the property does not exist in the current object, or the content of
	  the file is not valid for the format of the property type, then the
	  property is not matched.
- `PropertyName()`
	- Map the contents of selected files to the values of determined properties.
	- The property is determined by the base name of the file.
	- Files must be in formats supported by out.filter.PropertyName.
	- If a property does not exist in the current object, or the content of
	  the file is not valid for the format of the property type, then the file
	  is not matched.
- `Ignore()`
	- Ignore selected files.

## Full syntax

*I'm not familiar with BNF, but you should get the idea.*

```
<rule>     := <type> <func> `:` <func> `\n` ;
<type>     := `out` | `in` ;
<func>     := <word> `(` [ <argument> { `,` <argument> } ] `)` ;
<argument> := { `\` <any> | <any> - ( `,` | `)` ) } ;
<comment>  := `#` { <any> } `\n` ;
```

## Examples

```
# Write children to `children.rbxmx`
out Child(*) : File(children.rbxmx)

# Select children.rbxmx, read its content as a group of child objects.
in File(children.rbxmx) : Children()

# Write properties of all directory'd objects to `properties.json`
out Property(*, *) : File(properties.json)

# Select properties.json, read its content as a group of properties.
in File(properties.json) : Properties()

# Write certain containers as directories
out Child(Workspace) : Directory()
out Child(ServerStorage) : Directory()
out Child(Folder) : Directory()

# Write Source property to `source.lua`
out Property(*, Source, ProtectedString) : File(source.lua)

# Select source.lua, read its content as the value of the Source property.
in File(source.lua) : Property(Source)

# Write Terrain data to binary file
out Property(Terrain, *, BinaryString) : PropertyName(bin)

# Select .bin files, read their contents as the values of the properties named by the file name.
in File(*.bin) : PropertyName()
```
