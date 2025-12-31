include Makefile

APP_NAME			  := webplus-openapi-sync
IMAGE := hub.sudytech.cn/webplus/$(APP_NAME)
MAIN_FILE := table_sync_main.go

# 获取当前 Makefile 文件名（不带路径）
CURRENT_MAKEFILE_NAME := $(notdir $(firstword $(MAKEFILE_LIST)))
DOCKER_FILE:= table_sync.Dockerfile



