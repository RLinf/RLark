package task

import _ "embed"

//go:embed files/ray_head.sh
var rayHeadScript string

//go:embed files/ray_worker.sh
var rayWorkerScript string

//go:embed files/ray_check.py
var rayCheckScript string
