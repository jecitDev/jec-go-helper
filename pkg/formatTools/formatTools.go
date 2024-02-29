package formattools

import "fmt"

// Declare Medical Number
type MedicalNo struct {
	medicalNo uint64
}

// Generate New Medical Number
func NewMedicalNo(p_iMr uint64) *MedicalNo {
	return &MedicalNo{medicalNo: p_iMr}
}

// Medical Number Pattern
func (mr MedicalNo) String() string {
	x1 := mr.medicalNo / 1e4
	x2 := mr.medicalNo / 1e2 % 1e2
	x3 := mr.medicalNo % 1e2
	medicalNo := fmt.Sprintf("%03d-%02d-%02d", x1, x2, x3)

	return medicalNo
}
