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
	ErrNilNode     = errors.New("node cannot be nil")
	ErrSameDir     = errors.New("parent and child cannot be the same")
)

// Node is the tree node
type Node[T comparable] struct {
	Value           *T
	Children        map[string]*Node[T]
	ChildrenByInode map[uint64]*Node[T]
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
		node.ChildrenByInode = make(map[uint64]*Node[T])
	}
	return &node
}

func (node *Node[T]) HasChildren() bool {
	return (len(node.Children) > 0 || len(node.ChildrenByInode) > 0)
}

func NewTree[T comparable]() Tree[T] {
	tree := Tree[T]{
		Root: nil,
	}
	return tree
}

func (tree *Tree[T]) getNodeByName(pathName string) (retNode *Node[T], retError error) {
	var ok bool
	var v *Node[T]

	if tree.Root == nil {
		panic("root node not found")
	}

	if pathName == "." || pathName == "/" {
		retNode = tree.Root
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
		retNode = v
		return
	} else {
		retError = ErrNotFound
		return
	}
}

func (tree *Tree[T]) getNodeByInode(inode uint64, startNode, foundNode *Node[T]) (*Node[T], error) {
	var ok bool

	if foundNode != nil {
		return foundNode, nil
	}

	if startNode == nil && foundNode == nil {
		return nil, ErrNotFound
	}

	if startNode.HasChildren() {
		foundNode, ok = startNode.ChildrenByInode[inode]
		if ok {
			return foundNode, nil
		}
		for _, node := range startNode.ChildrenByInode {
			if node.HasChildren() {
				return tree.getNodeByInode(inode, node, foundNode)
			}
		}
	}
	return nil, ErrNotFound
}

func (tree *Tree[T]) Get(pathName string) (*Node[T], error) {
	return tree.getNodeByName(pathName)
}

func (tree *Tree[T]) ListByName(pathName string) ([]*T, error) {

	var children map[string]*Node[T]
	var retList []*T

	if tree.Root == nil {
		panic("root node not found")
	}

	retList = make([]*T, 0)
	if pathName == "." || pathName == "/" {
		children = tree.Root.Children
	} else {
		node, err := tree.getNodeByName(pathName)
		if err != nil {
			return nil, err
		}
		if len(node.Children) == 0 {
			return nil, ErrNoChildren
		}
		children = node.Children
	}
	for name, v := range children {
		if name == "." || name == ".." {
			continue
		}
		retList = append(retList, v.Value)
	}
	return retList, nil
}

// TODO implement this method for a Node[T] to make searches
// from the given position and faster.
func (tree *Tree[T]) ListByInode(inode uint64) ([]*T, error) {

	var children map[string]*Node[T]
	var retList []*T

	if tree.Root == nil {
		panic("root node not found")
	}

	retList = make([]*T, 0)
	node, err := tree.getNodeByInode(inode, tree.Root, nil)
	if err != nil {
		return nil, err
	}
	if node.HasChildren() {
		children = node.Children
		for name, v := range children {
			if name == "." || name == ".." {
				continue
			}
			retList = append(retList, v.Value)
		}
		return retList, nil
	}
	return nil, ErrNoChildren
}

func (tree *Tree[T]) Add(pathName string, parentNode, childNode *Node[T], pInode, cInode uint64, children bool) error {

	if tree.Root == nil {
		if !children {
			panic("adding root with no children")
		}
		parentNode.Children["."] = parentNode
		parentNode.Children[".."] = parentNode
		parentNode.ChildrenByInode[pInode] = parentNode
		tree.Root = parentNode
		return nil
	}
	if parentNode == nil || childNode == nil {
		return ErrNilNode
	}
	if parentNode == childNode {
		return ErrSameDir
	}
	baseName := path.Base(pathName)
	parentNode.Children[baseName] = childNode
	parentNode.ChildrenByInode[cInode] = childNode
	if children {
		childNode.Children["."] = childNode
		childNode.Children[".."] = parentNode
		childNode.ChildrenByInode[cInode] = childNode
		childNode.ChildrenByInode[pInode] = parentNode
	}
	return nil
}
