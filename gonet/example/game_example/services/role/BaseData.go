package role

import "fmt"

const (
	GenderUnset  = 0
	GenderMale   = 1
	GenderFemale = 2
)

// BaseData 是玩家基础资料。由 load_data 装入，其它模块通过方法读写，不要直接改字段。
type BaseData struct {
	level          int
	name           string
	gender         int
	lastLoginTime  int64
	lastLogoutTime int64
	onChange       func()
}

func NewBaseData(alias string) *BaseData {
	return &BaseData{
		level:  1,
		name:   alias,
		gender: GenderUnset,
	}
}

func (b *BaseData) bindChange(fn func()) {
	if b == nil {
		return
	}
	b.onChange = fn
}

func (b *BaseData) changed() {
	if b != nil && b.onChange != nil {
		b.onChange()
	}
}

func (b *BaseData) Level() int {
	if b == nil {
		return 0
	}
	return b.level
}

func (b *BaseData) SetLevel(level int) error {
	if b == nil {
		return fmt.Errorf("basedata: nil")
	}
	if level < 1 {
		return fmt.Errorf("basedata: level 必须 >= 1")
	}
	b.level = level
	b.changed()
	return nil
}

func (b *BaseData) AddLevel(delta int) error {
	if b == nil {
		return fmt.Errorf("basedata: nil")
	}
	return b.SetLevel(b.level + delta)
}

func (b *BaseData) Name() string {
	if b == nil {
		return ""
	}
	return b.name
}

func (b *BaseData) SetName(name string) error {
	if b == nil {
		return fmt.Errorf("basedata: nil")
	}
	if name == "" {
		return fmt.Errorf("basedata: name 不能为空")
	}
	b.name = name
	b.changed()
	return nil
}

func (b *BaseData) Gender() int {
	if b == nil {
		return GenderUnset
	}
	return b.gender
}

func (b *BaseData) LastLoginTime() int64 {
	if b == nil {
		return 0
	}
	return b.lastLoginTime
}

// SetLastLoginTime 记下最近一次登录，unix 秒。不走资料变更通知，调用方自己 markDirty。
func (b *BaseData) SetLastLoginTime(t int64) {
	if b == nil {
		return
	}
	b.lastLoginTime = t
}

func (b *BaseData) LastLogoutTime() int64 {
	if b == nil {
		return 0
	}
	return b.lastLogoutTime
}

// SetLastLogoutTime 记下最近一次下线，unix 秒。不走资料变更通知，调用方自己 markDirty。
func (b *BaseData) SetLastLogoutTime(t int64) {
	if b == nil {
		return
	}
	b.lastLogoutTime = t
}

func (b *BaseData) SetGender(gender int) error {
	if b == nil {
		return fmt.Errorf("basedata: nil")
	}
	switch gender {
	case GenderUnset, GenderMale, GenderFemale:
		b.gender = gender
		b.changed()
		return nil
	default:
		return fmt.Errorf("basedata: gender 无效")
	}
}
