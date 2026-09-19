package pickup

import (
	"sort"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Age-group keys reported in participant statistics.
const (
	AgeGroupUnder18 = "under_18"
	AgeGroup18To24  = "18_24"
	AgeGroup25To34  = "25_34"
	AgeGroup35To44  = "35_44"
	AgeGroup45To54  = "45_54"
	AgeGroup55Plus  = "55_plus"
	AgeGroupUnknown = "unknown"
)

// GenderUnknown is reported for seats whose gender was never provided.
const GenderUnknown = "unknown"

var (
	genderOrder   = []string{user.GenderMale, user.GenderFemale, user.GenderOther, GenderUnknown}
	ageGroupOrder = []string{AgeGroupUnder18, AgeGroup18To24, AgeGroup25To34, AgeGroup35To44, AgeGroup45To54, AgeGroup55Plus, AgeGroupUnknown}
)

// ParticipantSeat is one occupied seat, reduced to the attributes that are
// aggregated into statistics. It carries no identity.
type ParticipantSeat struct {
	Gender     *string
	BirthDate  *time.Time // nil for anonymous party members and users without a birth date
	SkillLevel int
}

// GenderCount is the number of seats of one gender.
type GenderCount struct {
	Gender string
	Count  int
}

// AgeGroupCount is the number of seats in one age group.
type AgeGroupCount struct {
	Group string
	Count int
}

// SkillLevelCount is the number of seats at one self-reported skill level.
type SkillLevelCount struct {
	Level int
	Label string // Empty when the sport has no mapping row for the level
	Count int
}

// ParticipantStats is the anonymous demographic breakdown of a group's enrolled
// seats (pending and confirmed orders).
type ParticipantStats struct {
	Total       int
	Genders     []GenderCount
	AgeGroups   []AgeGroupCount
	SkillLevels []SkillLevelCount
}

// AgeGroupOf buckets an age in whole years.
func AgeGroupOf(age int) string {
	switch {
	case age < 18:
		return AgeGroupUnder18
	case age <= 24:
		return AgeGroup18To24
	case age <= 34:
		return AgeGroup25To34
	case age <= 44:
		return AgeGroup35To44
	case age <= 54:
		return AgeGroup45To54
	default:
		return AgeGroup55Plus
	}
}

// BuildParticipantStats aggregates seats into statistics. now is the instant the
// ages are computed at (already converted to the zone whose calendar date
// should be used), and labels maps a level to its display label for the group's
// sport. Every gender and age group is always present (with a zero count); the
// skill-level list contains every level of the sport plus any level seen in
// the data, in ascending order.
func BuildParticipantStats(seats []ParticipantSeat, now time.Time, labels map[int]string) ParticipantStats {
	genders := make(map[string]int)
	ages := make(map[string]int)
	levels := make(map[int]int)

	for _, s := range seats {
		g := GenderUnknown
		if s.Gender != nil && user.IsValidGender(*s.Gender) {
			g = *s.Gender
		}
		genders[g]++

		ag := AgeGroupUnknown
		if s.BirthDate != nil {
			age := user.AgeOn(*s.BirthDate, now)
			if age < 0 {
				age = 0
			}
			ag = AgeGroupOf(age)
		}
		ages[ag]++

		levels[s.SkillLevel]++
	}

	stats := ParticipantStats{Total: len(seats)}
	for _, g := range genderOrder {
		stats.Genders = append(stats.Genders, GenderCount{Gender: g, Count: genders[g]})
	}
	for _, ag := range ageGroupOrder {
		stats.AgeGroups = append(stats.AgeGroups, AgeGroupCount{Group: ag, Count: ages[ag]})
	}

	seen := make(map[int]struct{}, len(labels)+len(levels))
	var ordered []int
	for l := range labels {
		seen[l] = struct{}{}
		ordered = append(ordered, l)
	}
	for l := range levels {
		if _, ok := seen[l]; !ok {
			ordered = append(ordered, l)
		}
	}
	sort.Ints(ordered)
	for _, l := range ordered {
		stats.SkillLevels = append(stats.SkillLevels, SkillLevelCount{Level: l, Label: labels[l], Count: levels[l]})
	}
	return stats
}
