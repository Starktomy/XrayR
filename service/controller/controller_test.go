package controller

import (
	"reflect"
	"testing"

	"github.com/Starktomy/XrayR/api"
)

func TestCompareUserListEmpty(t *testing.T) {
	old := &[]api.UserInfo{}
	new := &[]api.UserInfo{}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 0 || len(added) != 0 {
		t.Errorf("expected no diff on empty input, got deleted=%v added=%v", deleted, added)
	}
}

func TestCompareUserListAllAdded(t *testing.T) {
	old := &[]api.UserInfo{}
	new := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
	}
	_, added := compareUserList(old, new)
	if !reflect.DeepEqual(added, *new) {
		t.Errorf("expected all-new users to be reported as added, got %v", added)
	}
}

func TestCompareUserListAllDeleted(t *testing.T) {
	old := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
	}
	new := &[]api.UserInfo{}
	deleted, _ := compareUserList(old, new)
	if !reflect.DeepEqual(deleted, *old) {
		t.Errorf("expected all-old users to be reported as deleted, got %v", deleted)
	}
}

func TestCompareUserListNoChange(t *testing.T) {
	users := []api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x"},
	}
	old := &users
	new := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x"},
	}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 0 || len(added) != 0 {
		t.Errorf("expected no diff on identical input, got deleted=%v added=%v", deleted, added)
	}
}

func TestCompareUserListMixedDiff(t *testing.T) {
	old := &[]api.UserInfo{
		{UID: 1, Email: "alice@x", SpeedLimit: 100},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x", SpeedLimit: 100},
	}
	new := &[]api.UserInfo{
		// alice unchanged
		{UID: 1, Email: "alice@x", SpeedLimit: 100},
		// bob removed
		// carol's SpeedLimit changed -> should appear as
		// delete + add (matches pre-existing behaviour)
		{UID: 3, Email: "carol@x", SpeedLimit: 200},
		// dan added
		{UID: 4, Email: "dan@x"},
	}
	deleted, added := compareUserList(old, new)
	wantDeleted := []api.UserInfo{
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x", SpeedLimit: 100},
	}
	wantAdded := []api.UserInfo{
		{UID: 3, Email: "carol@x", SpeedLimit: 200},
		{UID: 4, Email: "dan@x"},
	}
	if !reflect.DeepEqual(deleted, wantDeleted) {
		t.Errorf("deleted: got %v, want %v", deleted, wantDeleted)
	}
	if !reflect.DeepEqual(added, wantAdded) {
		t.Errorf("added: got %v, want %v", added, wantAdded)
	}
}

func TestCompareUserListUnsorted(t *testing.T) {
	// The algorithm sorts internally, so input order must
	// not matter. Provide both lists in a different order
	// than the algorithm would produce and confirm the diff
	// is the same.
	old := &[]api.UserInfo{
		{UID: 3, Email: "carol@x"},
		{UID: 1, Email: "alice@x"},
	}
	new := &[]api.UserInfo{
		{UID: 4, Email: "dan@x"},
		{UID: 1, Email: "alice@x"},
	}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 1 || deleted[0].UID != 3 {
		t.Errorf("deleted: got %v", deleted)
	}
	if len(added) != 1 || added[0].UID != 4 {
		t.Errorf("added: got %v", added)
	}
}
