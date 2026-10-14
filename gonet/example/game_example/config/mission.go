package config

// MissionDef 是全服静态任务定义，不进玩家库。
type MissionDef struct {
	ID          int
	Kind        string
	Target      int
	PreID       int
	RewardLevel int // 领取时 AddLevel 的增量，0 表示不发等级
}

const MissionKindLevel = "level"

var missionDefs = []MissionDef{
	{ID: 1001, Kind: MissionKindLevel, Target: 2, PreID: 0, RewardLevel: 1},
	{ID: 1002, Kind: MissionKindLevel, Target: 3, PreID: 1001, RewardLevel: 1},
	{ID: 1003, Kind: MissionKindLevel, Target: 5, PreID: 1002, RewardLevel: 1},
}

func MissionDefs() []MissionDef {
	out := make([]MissionDef, len(missionDefs))
	copy(out, missionDefs)
	return out
}

func LookupMission(id int) (MissionDef, bool) {
	for _, d := range missionDefs {
		if d.ID == id {
			return d, true
		}
	}
	return MissionDef{}, false
}
