package persistence

import "fmt"

func NextTargetID(st *State) string {
	id := fmt.Sprintf("TGT-%06d", st.NextTarget)
	st.NextTarget++
	return id
}

func NextJobID(st *State) string {
	id := fmt.Sprintf("JOB-%06d", st.NextJob)
	st.NextJob++
	return id
}

func NextSolutionID(st *State) string {
	id := fmt.Sprintf("SOL-%06d", st.NextSolution)
	st.NextSolution++
	return id
}

func VersionKey(targetID string, version int) string {
	return fmt.Sprintf("%s:%06d", targetID, version)
}

func ArcKey(stationID, arcID string) string {
	return stationID + ":" + arcID
}
