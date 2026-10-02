package main

import "fmt"

func inspectLineageGroup(m manifest, id string) error {
	for _, group := range m.Groups {
		if group.ID != id {
			continue
		}
		var poseID, sceneID string
		if style, ok := findSourceImage(m.StyleImages, group.StyleImageID); ok {
			poseID = style.PoseID
		}
		if frame, ok := findSourceImage(m.FirstFrames, group.FirstFrameID); ok {
			sceneID = frame.SceneID
		}
		fmt.Printf("source group: id=%s kind=%s state=%s next=%s model=%s style=%s frame=%s product=%s virtual_ip=%s pose=%s scene=%s videos=%d\n",
			group.ID, group.Kind, group.ExecutionState, group.NextStepKey, group.ModelImageID,
			group.StyleImageID, group.FirstFrameID, group.ProductID, group.VirtualIPID, poseID, sceneID, len(group.VideoIDs))
		return nil
	}
	return fmt.Errorf("source lineage group %q not found", id)
}
