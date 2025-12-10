package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/db"
	"github.com/stretchr/testify/assert"
	"github.com/z46-dev/gomysql"
)

func TestDBCrudSimpleContainer(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err error
		ct  *db.Container = &db.Container{
			PVEID:       123,
			IPAddress:   "10.10.10.10",
			Status:      "running",
			LastUpdated: time.Now(),
			CreatedAt:   time.Now(),
		}
	)

	// Create
	if err = db.Containers.Insert(ct); err != nil {
		t.Fatalf("failed to insert container: %v", err)
	}

	// Read
	var readCt *db.Container
	if readCt, err = db.Containers.Select(ct.PVEID); err != nil {
		t.Fatalf("failed to get container by ID: %v", err)
	}

	assert.NotNil(t, readCt)
	assert.Equal(t, ct.PVEID, readCt.PVEID)

	// Update
	ct.Status = "stopped"
	if err = db.Containers.Update(ct); err != nil {
		t.Fatalf("failed to update container: %v", err)
	}

	if readCt, err = db.Containers.Select(ct.PVEID); err != nil {
		t.Fatalf("failed to get container by ID after update: %v", err)
	}

	assert.Equal(t, "stopped", readCt.Status)

	// Delete
	if err = db.Containers.Delete(ct.PVEID); err != nil {
		t.Fatalf("failed to delete container: %v", err)
	}

	if readCt, err = db.Containers.Select(ct.PVEID); err != nil {
		t.Fatalf("failed to get container by ID after delete: %v", err)
	}

	assert.Nil(t, readCt)
}

func TestDBCrudManyContainer(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err        error
		numEntries = 100
		containers = make([]*db.Container, numEntries)
	)

	// Create many containers
	for i := range numEntries {
		containers[i] = &db.Container{
			PVEID:     int64(1000 + i),
			IPAddress: fmt.Sprintf("10.10.10.%d", i),
			Status: func() string {
				if i%2 == 0 {
					return "running"
				}

				return "stopped"
			}(),
			LastUpdated: time.Now(),
			CreatedAt:   time.Now(),
		}

		if err = db.Containers.Insert(containers[i]); err != nil {
			t.Fatalf("failed to insert container %d: %v", i, err)
		}
	}

	// Read and verify all containers
	for i := range numEntries {
		var readCt *db.Container
		if readCt, err = db.Containers.Select(containers[i].PVEID); err != nil {
			t.Fatalf("failed to get container %d by ID: %v", i, err)
		}

		assert.NotNil(t, readCt)
		assert.Equal(t, containers[i].PVEID, readCt.PVEID)
	}

	// Select all
	var allContainers []*db.Container
	if allContainers, err = db.Containers.SelectAll(); err != nil {
		t.Fatalf("failed to select all containers: %v", err)
	}

	assert.Equal(t, numEntries, len(allContainers))

	// Select running containers
	var runningContainers []*db.Container
	if runningContainers, err = db.Containers.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(db.Containers.FieldBySQLName("status"), gomysql.OpEqual, "running")); err != nil {
		t.Fatalf("failed to select running containers: %v", err)
	}

	var expectedRunning int
	for i := range numEntries {
		if containers[i].Status == "running" {
			expectedRunning++
		}
	}

	assert.Equal(t, expectedRunning, len(runningContainers))

	// Clean up
	for i := range numEntries {
		if err = db.Containers.Delete(containers[i].PVEID); err != nil {
			t.Fatalf("failed to delete container %d: %v", i, err)
		}
	}
}
