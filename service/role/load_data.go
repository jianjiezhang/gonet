package role

import (
	"context"
	"errors"
	"fmt"
	"time"

	"game/store"
)

var (
	ErrEmptyAlias = store.ErrEmptyAlias
	ErrNotFound   = store.ErrNotFound
	ErrExists     = store.ErrExists
	errNoStore    = errors.New("role: store 未初始化")
)

const dbTimeout = 5 * time.Second

type Data struct {
	Base     *BaseData
	Missions *Missions
}

func useStore() (store.Store, error) {
	s := store.Get()
	if s == nil {
		return nil, errNoStore
	}
	return s, nil
}

func HasRole(alias string) (bool, error) {
	db, err := useStore()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.HasRole(ctx, alias)
}

func CreateRole(alias string) error {
	db, err := useStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.CreateRole(ctx, alias)
}

func LoadData(alias string) (*Data, error) {
	db, err := useStore()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	row, err := db.LoadRole(ctx, alias)
	if err != nil {
		return nil, err
	}
	blob, err := db.LoadMission(ctx, alias)
	if err != nil {
		return nil, err
	}
	return dataFromStore(row, blob)
}

func SaveRole(row store.RoleRow) error {
	db, err := useStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.SaveRole(ctx, row)
}

func saveRoleTimeout(d time.Duration, row store.RoleRow) error {
	db, err := useStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return db.SaveRole(ctx, row)
}

func SaveMission(blob store.MissionBlob) error {
	db, err := useStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.SaveMission(ctx, blob)
}

func saveMissionTimeout(d time.Duration, blob store.MissionBlob) error {
	db, err := useStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return db.SaveMission(ctx, blob)
}

func dataFromStore(row store.RoleRow, blob store.MissionBlob) (*Data, error) {
	missions, err := newMissions(blob.Data)
	if err != nil {
		return nil, err
	}
	return &Data{
		Base: &BaseData{
			level:          row.Level,
			name:           row.Name,
			gender:         row.Gender,
			lastLoginTime:  row.LastLoginTime,
			lastLogoutTime: row.LastLogoutTime,
		},
		Missions: missions,
	}, nil
}

func (a *Actor) snapshot() (store.RoleRow, store.MissionBlob, error) {
	if a == nil || a.data == nil || a.data.Base == nil {
		return store.RoleRow{}, store.MissionBlob{}, fmt.Errorf("role: 无数据")
	}
	id := RoleID(a.SelfName())
	if id == "" {
		return store.RoleRow{}, store.MissionBlob{}, ErrEmptyAlias
	}
	blob, err := a.data.Missions.Blob(id)
	if err != nil {
		return store.RoleRow{}, store.MissionBlob{}, err
	}
	return store.RoleRow{
		RoleID:         id,
		Level:          a.data.Base.Level(),
		Name:           a.data.Base.Name(),
		Gender:         a.data.Base.Gender(),
		LastLoginTime:  a.data.Base.LastLoginTime(),
		LastLogoutTime: a.data.Base.LastLogoutTime(),
	}, blob, nil
}
