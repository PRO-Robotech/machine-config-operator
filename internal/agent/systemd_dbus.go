//go:build linux
// +build linux

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agent

import (
	"context"
	"fmt"

	"github.com/coreos/go-systemd/v22/dbus"
)

// DBusConnection implements SystemdConnection using go-systemd/dbus.
type DBusConnection struct {
	conn *dbus.Conn
}

// NewDBusConnection creates a new D-Bus connection to systemd.
func NewDBusConnection(ctx context.Context) (*DBusConnection, error) {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to systemd: %w", err)
	}
	return &DBusConnection{conn: conn}, nil
}

// Close closes the D-Bus connection.
func (c *DBusConnection) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// GetUnitProperty gets a property of a unit.
func (c *DBusConnection) GetUnitProperty(ctx context.Context, unit, property string) (interface{}, error) {
	props, err := c.conn.GetUnitPropertiesContext(ctx, unit)
	if err != nil {
		return nil, err
	}
	return props[property], nil
}

// MaskUnit masks a unit.
func (c *DBusConnection) MaskUnit(ctx context.Context, name string) error {
	_, err := c.conn.MaskUnitFilesContext(ctx, []string{name}, false, true)
	return err
}

// UnmaskUnit unmasks a unit.
func (c *DBusConnection) UnmaskUnit(ctx context.Context, name string) error {
	_, err := c.conn.UnmaskUnitFilesContext(ctx, []string{name}, false)
	return err
}

// EnableUnit enables a unit.
func (c *DBusConnection) EnableUnit(ctx context.Context, name string) error {
	_, _, err := c.conn.EnableUnitFilesContext(ctx, []string{name}, false, true)
	return err
}

// DisableUnit disables a unit.
func (c *DBusConnection) DisableUnit(ctx context.Context, name string) error {
	_, err := c.conn.DisableUnitFilesContext(ctx, []string{name}, false)
	return err
}

// StartUnit starts a unit and waits for completion.
func (c *DBusConnection) StartUnit(ctx context.Context, name string) error {
	ch := make(chan string, 1)
	_, err := c.conn.StartUnitContext(ctx, name, "replace", ch)
	if err != nil {
		return err
	}
	<-ch
	return nil
}

// StopUnit stops a unit and waits for completion.
func (c *DBusConnection) StopUnit(ctx context.Context, name string) error {
	ch := make(chan string, 1)
	_, err := c.conn.StopUnitContext(ctx, name, "replace", ch)
	if err != nil {
		return err
	}
	<-ch
	return nil
}

// RestartUnit restarts a unit and waits for completion.
func (c *DBusConnection) RestartUnit(ctx context.Context, name string) error {
	ch := make(chan string, 1)
	_, err := c.conn.RestartUnitContext(ctx, name, "replace", ch)
	if err != nil {
		return err
	}
	<-ch
	return nil
}

// ReloadUnit reloads a unit's configuration and waits for completion.
func (c *DBusConnection) ReloadUnit(ctx context.Context, name string) error {
	ch := make(chan string, 1)
	_, err := c.conn.ReloadUnitContext(ctx, name, "replace", ch)
	if err != nil {
		return err
	}
	<-ch
	return nil
}
