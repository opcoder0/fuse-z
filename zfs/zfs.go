package zfs

import (
	"errors"
	"path"
	"strings"
)

var (
	ErrNoChildren  = errors.New("no child nodes")
	ErrInvalidPath = errors.New("invalid path")
	ErrNotFound    = errors.New("name not found")
)

// Node is the tree node
type Node[T comparable] struct {
	Value    *T
	Children map[string]*Node[T]
}

type Tree[T comparable] struct {
	Root *Node[T]
}

func NewNode[T comparable](v *T, hasChildren bool) *Node[T] {
	node := Node[T]{
		Value: v,
	}
	if hasChildren {
		node.Children = make(map[string]*Node[T])
	}
	return &node
}

func NewTree[T comparable](v *T) Tree[T] {
	rootNode := NewNode[T](v, true)
	tree := Tree[T]{
		Root: rootNode,
	}
	return tree
}

func (tree Tree[T]) Get(pathName string) (ret *T, retError error) {
	// var name string
	var ok bool
	var v *Node[T]

	if tree.Root == nil {
		panic("root node not found")
	}

	// if strings.HasSuffix(pathName, "/") {
	// 	// directory
	// 	parent = path.Dir(path.Dir(pathName))
	// 	name = path.Base(pathName)
	// } else {
	// 	// file
	// 	parent = path.Dir(pathName)
	// 	name = path.Base(pathName)
	// }
	// if parent == "." || parent == "/" {
	// 	ret = tree.Root.Value
	// } else
	// {
	if pathName == "." || pathName == "/" {
		ret = tree.Root.Value
		return
	}
	parts := strings.Split(pathName, "/")
	v = tree.Root
	for _, part := range parts {
		if v.Children == nil {
			retError = ErrNoChildren
			return
		}
		v, ok = v.Children[part]
		if !ok {
			retError = ErrInvalidPath
			return
		}
	}
	if v != nil {
		ret = v.Value
		return
	} else {
		retError = ErrNotFound
		return
	}
	// }
	// return
}

func (tree *Tree[T]) Add(pathName string, value *T, children bool) error {

	var parent, name string
	var v *Node[T]
	var ok bool

	if tree.Root == nil {
		panic("root node not found")
	}
	if strings.HasSuffix(pathName, "/") {
		// directory
		parent = path.Dir(path.Dir(pathName))
		name = path.Base(pathName)
	} else {
		// file
		parent = path.Dir(pathName)
		name = path.Base(pathName)
	}
	newNode := NewNode[T](value, children)
	// TODO generalize the root name
	if parent == "." || parent == "/" {
		tree.Root.Children[name] = newNode
	} else {
		parts := strings.Split(parent, "/")
		v = tree.Root
		for _, part := range parts {
			if v.Children == nil {
				return ErrNoChildren
			}
			v, ok = v.Children[part]
			if !ok {
				return ErrInvalidPath
			}
		}
		if v != nil {
			v.Children[name] = newNode
		}
	}
	return nil
}
