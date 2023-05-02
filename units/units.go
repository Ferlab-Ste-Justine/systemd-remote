package units

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/Ferlab-Ste-Justine/systemd-remote/logger"

	"github.com/Ferlab-Ste-Justine/etcd-sdk/client"
	"github.com/coreos/go-systemd/v22/dbus"
	yaml "gopkg.in/yaml.v2"
)

const SYSTEMD_UNIT_FILES_PATH = "/etc/systemd/system"

var (
	ErrEmptyUnitName   = errors.New("Unit name was empty")
	ErrJobIsNotService = errors.New("Unit type other than .service was flagged as a job")
	ErrJobIsFlaggedOn  = errors.New("Job was flagged as on")
	ErrJobFlagChanged  = errors.New("Job flag was changed")
)

func IsApiContractError(err error) bool {
	return err == ErrEmptyUnitName || err == ErrJobIsNotService || err == ErrJobIsFlaggedOn || err == ErrJobFlagChanged
}

type Unit struct {
	Name string
	On   bool
	Job  bool
}

func (u *Unit) Validate() error {
	if u.Name == "" {
		return ErrEmptyUnitName
	}

	if (!strings.HasSuffix(u.Name, ".service")) && u.Job {
		return ErrJobIsNotService
	}

	if u.On && u.Job {
		return ErrJobIsFlaggedOn
	}

	return nil
}

func (u *Unit) IsPersistentService() bool {
	if strings.HasSuffix(u.Name, ".service") && u.Job {
		return false
	}

	return true
}

func (u *Unit) ServiceShould() string {
	if !u.IsPersistentService() {
		return ""
	}

	if u.On {
		return "start"
	}

	return "stop"
}

type Units map[string]Unit

type UnitsManager struct {
	FilePath string
	Logger   logger.Logger
	units    *Units
}

func IsPersistentService(unitName string, units *Units) bool {
	if val, ok := (*units)[unitName]; ok {
		return val.IsPersistentService()
	}

	return true
}

func ServiceShould(unitName string, units *Units) string {
	if val, ok := (*units)[unitName]; ok {
		return val.ServiceShould()
	}

	return ""
}

func IsRunningService(unitName string, unitsStatus map[string]dbus.UnitStatus) bool {
	if val, ok := unitsStatus[unitName]; ok {
		return val.ActiveState != "inactive" && val.ActiveState != "deactivating"
	}

	return false
}

func (man *UnitsManager) SaveUnitsConf(exists bool) error {
	if !exists {
		dir := path.Dir(man.FilePath)
		if dir != "" {
			mkErr := os.MkdirAll(dir, 0700)
			if mkErr != nil {
				return mkErr
			}
		}
	}

	buf := new(bytes.Buffer)
	enc := yaml.NewEncoder(buf)
	_ = enc.Encode(*(man.units))
	return ioutil.WriteFile(man.FilePath, buf.Bytes(), 0700)
}

func (man *UnitsManager) LoadUnitsConf() error {
	if man.units != nil {
		return nil
	}

	u := Units(map[string]Unit{})

	_, statErr := os.Stat(man.FilePath)
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}

		man.Logger.Infof("Units configuration file %s not found. Creating it.", man.FilePath)
		man.units = &u
		man.SaveUnitsConf(false)
	}

	bs, readErr := ioutil.ReadFile(man.FilePath)
	if readErr != nil {
		return readErr
	}

	yamlErr := yaml.Unmarshal(bs, &u)
	if yamlErr != nil {
		return yamlErr
	}

	man.units = &u
	return nil
}

func (man *UnitsManager) InsertUnits(conn *dbus.Conn, inserts map[string]string) error {
	if len(inserts) == 0 {
		return nil
	}

	unitsStatus, unitsStatusErr := getUnitsStatus(conn)
	if unitsStatusErr != nil {
		return unitsStatusErr
	}

	for key, val := range inserts {
		created := true
		if !strings.HasSuffix(key, ".service") && !strings.HasSuffix(key, ".timer") {
			man.Logger.Warnf("Unit file %s has unsupported extension. Skipping insert.", key)
			continue
		}

		unitPath := path.Join(SYSTEMD_UNIT_FILES_PATH, key)
		_, statErr := os.Stat(unitPath)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
		} else {
			man.Logger.Warnf("Insert: unit file %s exists. will update it instead.", key)
			created = false
		}

		if created {
			man.Logger.Infof("Creating unit file %s.", key)
		} else {
			man.Logger.Infof("Updating unit file %s.", key)
		}

		writeErr := ioutil.WriteFile(unitPath, []byte(val), 0640)
		if writeErr != nil {
			return writeErr
		}

		reloadErr := conn.Reload()
		if reloadErr != nil {
			return reloadErr
		}

		if IsPersistentService(key, man.units) && IsRunningService(key, unitsStatus) {
			man.Logger.Infof("Restarting service %s.", key)

			output := make(chan string)
			defer close(output)
			_, restartErr := conn.RestartUnit(key, "replace", output)
			if restartErr != nil {
				return restartErr
			}
			<-output
		}
	}

	return nil
}

func (man *UnitsManager) UpdateUnits(conn *dbus.Conn, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}

	unitsStatus, unitsStatusErr := getUnitsStatus(conn)
	if unitsStatusErr != nil {
		return unitsStatusErr
	}

	for key, val := range updates {
		created := false
		if !strings.HasSuffix(key, ".service") && !strings.HasSuffix(key, ".timer") {
			man.Logger.Warnf("Unit file %s has unsupported extension. Skipping update.", key)
			continue
		}

		unitPath := path.Join(SYSTEMD_UNIT_FILES_PATH, key)
		_, statErr := os.Stat(unitPath)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}

			man.Logger.Warnf("Update: unit file %s does not exist. will create it instead.", key)
			created = true
		}

		if created {
			man.Logger.Infof("Creating unit file %s.", key)
		} else {
			man.Logger.Infof("Updating unit file %s.", key)
		}

		writeErr := ioutil.WriteFile(unitPath, []byte(val), 0640)
		if writeErr != nil {
			return writeErr
		}

		reloadErr := conn.Reload()
		if reloadErr != nil {
			return reloadErr
		}

		if IsPersistentService(key, man.units) && IsRunningService(key, unitsStatus) {
			man.Logger.Infof("Restarting service %s.", key)

			output := make(chan string)
			defer close(output)
			_, restartErr := conn.RestartUnit(key, "replace", output)
			if restartErr != nil {
				return restartErr
			}
			<-output
		}
	}

	return nil
}

func (man *UnitsManager) DeleteUnits(conn *dbus.Conn, deletions []string) error {
	if len(deletions) == 0 {
		return nil
	}

	unitsStatus, unitsStatusErr := getUnitsStatus(conn)
	if unitsStatusErr != nil {
		return unitsStatusErr
	}

	for _, key := range deletions {
		unitPath := path.Join(SYSTEMD_UNIT_FILES_PATH, key)
		_, statErr := os.Stat(unitPath)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}

			man.Logger.Warnf("Delete: units configuration file %s not found. Skipping deletion.", key)
			continue
		}

		if IsRunningService(key, unitsStatus) {
			man.Logger.Infof("Stopping and disabling service %s.", key)

			output := make(chan string)
			defer close(output)
			_, stopErr := conn.StopUnit(key, "replace", output)
			if stopErr != nil {
				return stopErr
			}
			<-output

			_, disErr := conn.DisableUnitFilesContext(context.Background(), []string{key}, false)
			if disErr != nil {
				return disErr
			}
		}

		man.Logger.Infof("Removing unit file %s.", key)
		remErr := os.Remove(unitPath)
		if remErr != nil {
			return remErr
		}

		reloadErr := conn.Reload()
		if reloadErr != nil {
			return reloadErr
		}
	}

	return nil
}

func (man *UnitsManager) updateUnitStatus(conn *dbus.Conn, unitsStatus map[string]dbus.UnitStatus, old *Unit, new *Unit) error {
	var unitName string
	var action string
	if new == nil {
		//Insert
		unitName = new.Name
		action = new.ServiceShould()
	} else if new == nil {
		//Deletion
		unitName = old.Name
		action = ""
		if old.IsPersistentService() {
			action = "stop"
		}
	} else {
		//Update
		if new.IsPersistentService() != old.IsPersistentService() {
			return ErrJobFlagChanged
		}

		unitName = new.Name
		action = new.ServiceShould()
	}

	if action == "start" && (!IsRunningService(unitName, unitsStatus)) {
		man.Logger.Infof("Starting and enabling service %s.", unitName)

		_, _, enaErr := conn.EnableUnitFilesContext(context.Background(), []string{unitName}, false, true)
		if enaErr != nil {
			return enaErr
		}

		output := make(chan string)
		defer close(output)
		_, stopErr := conn.StartUnit(unitName, "replace", output)
		if stopErr != nil {
			return stopErr
		}
		<-output
	}

	if action == "stop" && IsRunningService(unitName, unitsStatus) {
		man.Logger.Infof("Stopping and disabling service %s.", unitName)

		output := make(chan string)
		defer close(output)
		_, stopErr := conn.StopUnit(unitName, "replace", output)
		if stopErr != nil {
			return stopErr
		}
		<-output

		_, disErr := conn.DisableUnitFilesContext(context.Background(), []string{unitName}, false)
		if disErr != nil {
			return disErr
		}
	}

	return nil
}

func (man *UnitsManager) ApplyUnitsConf(conn *dbus.Conn, newConf *Units) error {
	unitsStatus, unitsStatusErr := getUnitsStatus(conn)
	if unitsStatusErr != nil {
		return unitsStatusErr
	}

	for key, valOld := range *man.units {
		if valNew, ok := (*newConf)[key]; ok {
			//update
			err := valNew.Validate()
			if err != nil {
				return err
			}

			err = man.updateUnitStatus(conn, unitsStatus, &valOld, &valNew)
			if err != nil {
				return err
			}
			(*man.units)[key] = valNew
		} else {
			//delete
			err := man.updateUnitStatus(conn, unitsStatus, &valOld, nil)
			if err != nil {
				return err
			}
			delete(*man.units, key)
		}
	}

	for key, valNew := range *newConf {
		if _, ok := (*man.units)[key]; !ok {
			//insert
			err := valNew.Validate()
			if err != nil {
				return err
			}

			err = man.updateUnitStatus(conn, unitsStatus, nil, &valNew)
			if err != nil {
				return err
			}
			(*man.units)[key] = valNew
		}
	}

	return nil
}

func ExtractUnitsConfig(diff *client.KeyDiff) (*Units, error) {
	var u Units

	if val, ok := diff.Inserts["units.yml"]; ok {
		delete(diff.Inserts, "units.yml")
		yamlErr := yaml.Unmarshal([]byte(val), &u)
		return &u, yamlErr
	}

	if val, ok := diff.Updates["units.yml"]; ok {
		delete(diff.Updates, "units.yml")
		yamlErr := yaml.Unmarshal([]byte(val), &u)
		return &u, yamlErr
	}

	for idx, val := range diff.Deletions {
		if val == "units.yml" {
			diff.Deletions = append(diff.Deletions[0:idx], diff.Deletions[idx+1:]...)
			u = Units(map[string]Unit{})
			return &u, nil
		}
	}

	return nil, nil
}

func getUnitsStatus(conn *dbus.Conn) (map[string]dbus.UnitStatus, error) {
	result := map[string]dbus.UnitStatus{}

	statusList, listErr := conn.ListUnits()
	if listErr != nil {
		return result, listErr
	}

	for _, unit := range statusList {
		if strings.HasSuffix(unit.Name, ".service") || strings.HasSuffix(unit.Name, ".timer") {
			result[unit.Name] = unit
		}
	}

	return result, nil
}

func (man *UnitsManager) Apply(diff client.KeyDiff) error {
	conn, err := dbus.NewSystemdConnectionContext(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()

	newUnitsConf, err := ExtractUnitsConfig(&diff)
	if err != nil {
		return err
	}

	err = man.InsertUnits(conn, diff.Inserts)
	if err != nil {
		return err
	}

	err = man.UpdateUnits(conn, diff.Updates)
	if err != nil {
		return err
	}

	if newUnitsConf != nil {
		err = man.ApplyUnitsConf(conn, newUnitsConf)
		if err != nil {
			return err
		}

		err := man.SaveUnitsConf(true)
		if err != nil {
			return err
		}
	}

	err = man.DeleteUnits(conn, diff.Deletions)
	if err != nil {
		return err
	}

	return nil
}
