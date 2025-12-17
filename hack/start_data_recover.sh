#!/bin/bash

# -------------------------------
# 基本变量定义
# -------------------------------
CONTAINER_NAME="webplus-openapi-recover"
IMAGE_NAME="hub.sudytech.cn/webplus/webplus-openapi-recover"
IMAGE_TAG="2.0.0"
FULL_IMAGE_NAME="${IMAGE_NAME}:${IMAGE_TAG}"

# 主机路径
HOST_ETC_DIR="/opt/sudytech/webplus-openapi/etc"
HOST_CONFIG_FILE="${HOST_ETC_DIR}/config.yaml"
HOST_DATA_DIR="${HOST_ETC_DIR}/data"

# 容器内路径（固定）
CONTAINER_CONFIG_PATH="/app/etc/config/config.yaml"
CONTAINER_DATA_PATH="/app/etc/data"

# -------------------------------
# 彩色输出函数
# -------------------------------
print_info()  { echo -e "\033[1;32m[INFO]\033[0m  $1"; }
print_warn()  { echo -e "\033[1;33m[WARN]\033[0m  $1"; }
print_error() { echo -e "\033[1;31m[ERROR]\033[0m $1"; }

# -------------------------------
# 检查配置和数据目录
# -------------------------------
check_env() {
    if [ ! -f "${HOST_CONFIG_FILE}" ]; then
        print_error "配置文件不存在: ${HOST_CONFIG_FILE}"
        exit 1
    fi
    if [ ! -d "${HOST_DATA_DIR}" ]; then
        print_warn "数据目录不存在，自动创建: ${HOST_DATA_DIR}"
        mkdir -p "${HOST_DATA_DIR}"
    fi
}

# -------------------------------
# 检查镜像是否存在
# -------------------------------
ensure_image() {
    if ! docker image inspect "${FULL_IMAGE_NAME}" >/dev/null 2>&1; then
        print_warn "镜像 ${FULL_IMAGE_NAME} 不存在，开始拉取..."
        docker pull "${FULL_IMAGE_NAME}"
        # shellcheck disable=SC2181
        if [ $? -ne 0 ]; then
            print_error "镜像拉取失败！"
            exit 1
        fi
    else
        print_info "镜像 ${FULL_IMAGE_NAME} 已存在。"
    fi
}

# -------------------------------
# 启动容器执行修复任务
# -------------------------------
start_container() {
    print_info "开始执行 Webplus OpenAPI Recover 任务..."

    # 删除旧容器
    if docker ps -a --format '{{.Names}}' | grep -wq "${CONTAINER_NAME}"; then
        print_warn "检测到旧容器，正在删除..."
        docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1
    fi

    ensure_image

    print_info "启动容器执行修复任务（运行完毕后容器将自动退出）..."

    docker run --name -d "${CONTAINER_NAME}" \
        -v "${HOST_CONFIG_FILE}:${CONTAINER_CONFIG_PATH}" \
        -v "${HOST_DATA_DIR}:${CONTAINER_DATA_PATH}" \
        "${FULL_IMAGE_NAME}" \

    # shellcheck disable=SC2181
    if [ $? -eq 0 ]; then
        print_info "修复任务执行正在执行中。"
    else
        print_error "修复任务执行失败，请检查日志。"
    fi
}

# -------------------------------
# 查看日志（若正在运行）
# -------------------------------
show_logs() {
    if docker ps --format '{{.Names}}' | grep -wq "${CONTAINER_NAME}"; then
        docker logs -f "${CONTAINER_NAME}"
    else
        print_warn "当前没有正在运行的 ${CONTAINER_NAME} 容器。"
    fi
}

# -------------------------------
# 脚本帮助信息
# -------------------------------
show_help() {
    echo "======================================================="
    echo " Webplus OpenAPI Recover 管理脚本"
    echo "======================================================="
    echo "用法:"
    echo "  $0 start       启动修复任务（自动执行完退出）"
    echo "  $0 logs        查看运行日志（如果任务仍在执行）"
    echo "  $0 help        显示帮助信息"
    echo
    echo "示例:"
    echo "  bash $0 start"
    echo
    echo "配置挂载:"
    echo "  ${HOST_CONFIG_FILE}  →  /app/etc/config/config.yaml"
    echo "  ${HOST_DATA_DIR}     →  /app/etc/data"
    echo "======================================================="
}

# -------------------------------
# 主执行逻辑
# -------------------------------
case "$1" in
    start)
        check_env
        start_container
        ;;
    logs)
        show_logs
        ;;
    help|*)
        show_help
        ;;
esac
