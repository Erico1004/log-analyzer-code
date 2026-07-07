package main

import (
	"fmt"
	"log"
	"os"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/model"
)

func main() {
	config.LoadConfig()
	if err := database.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 清空旧数据
	database.DB.Exec("DELETE FROM knowledge_base")
	database.DB.Exec("ALTER TABLE knowledge_base AUTO_INCREMENT = 1")
	fmt.Println("✅ 旧知识库已清空")

	entries := []model.KnowledgeBase{
		// ────────── OOM 类 ──────────
		{
			Title:    "B2ts_v2 VM 内存耗尽触发 OOM Killer 终止日志转发进程",
			Category: "OOM",
			Content: `【根因】B2ts_v2 虚拟机内存耗尽，Linux 内核 OOM Killer 介入，将 haproxy 子进程 vector（日志转发组件）标记为高 oom_score 并强制终止以释放内存。
【触发条件】系统日志中出现 "Under memory pressure, flushing caches" 表明已进入严重内存压力状态。
【影响】日志转发中断，导致下游监控数据缺失。
【修复步骤】1) 扩容 VM 内存或增加 swap；2) 调整 vector 进程的 oom_score_adj 降低被杀优先级；3) 检查 haproxy 是否有内存泄漏；4) 配置 systemd MemoryLimit 限制。`,
			Keywords: "OOM, OOM Killer, 内存耗尽, B2ts_v2, vector, haproxy, memory pressure, flushing caches, swap, 进程终止, Linux内核",
			Symptoms: "oom-killer, oom_score_adj, invoked oom-killer, memory pressure, flushing caches, vector, haproxy, B2ts_v2, score 980, gfp_mask, systemd-journald, sacrifice child, Kill process",
		},
		{
			Title:    "Trello 业务高峰期 Worker OOM 崩溃与 Auto Scaling 策略失效",
			Category: "OOM",
			Content: `【根因】Trello 服务在业务高峰期 Worker 进程内存超限，触发 OOM 崩溃。基于 CPU 利用率的自动扩展策略未能及时感知内存压力，导致扩容延迟。
【触发条件】日志显示 "Worker process exceeded memory limit" 且 "CPU threshold not met while memory exhausted"。
【修复步骤】1) 增加 Trello Worker 的内存配额（JVM -Xmx 或容器 memory limit）；2) Auto Scaling 策略增加内存利用率指标（不仅是 CPU）；3) 配置 HPA 使用 memory 指标或 custom metrics。`,
			Keywords: "OOM, Trello, Worker, 内存超限, Auto Scaling, CPU threshold, memory limit, HPA, 业务高峰",
			Symptoms: "Trello, Worker process exceeded memory limit, Out-Of-Memory, memory exhausted, Scaling failed, CPU threshold not met, memory exhausted",
		},
		{
			Title:    "Windows Update KB5073722 导致 TiWorker.exe 内存泄漏引发 NTFS 损坏",
			Category: "OOM",
			Content: `【根因】安装 2026 年 1 月 Windows 累积更新 KB5073722 时，TiWorker.exe（Windows 更新模块）发生内存泄漏（RADAR_PRE_LEAK_64 检测），内存耗尽后 NTFS 文件系统索引 ($I30:$INDEX_ALLOCATION) 损坏。
【触发条件】Windows 更新日志中出现 RADAR_PRE_LEAK_64 警告，随后出现 NTFS Event ID 55。
【修复步骤】1) 卸载 KB5073722 补丁；2) 运行 chkdsk /f 修复 NTFS 索引；3) 在安全模式下运行 sfc /scannow；4) 等待微软发布修复补丁后再安装。`,
			Keywords: "OOM, TiWorker, KB5073722, 内存泄漏, RADAR_PRE_LEAK_64, NTFS, Windows Update, CHKDSK, 文件系统损坏",
			Symptoms: "KB5073722, TiWorker, RADAR_PRE_LEAK_64, NTFS, corruption, Event ID 55, $I30, INDEX_ALLOCATION, Memory corruption, file system error, Installing KB",
		},

		// ────────── 网络类 ──────────
		{
			Title:    "Cisco CBS250 DNS 客户端缺陷导致核心转储和反复重启",
			Category: "网络",
			Content: `【根因】Cisco CBS250 交换机的 DNS 客户端在处理外部域名解析失败时触发内部缺陷（bug），DNS 客户端进程崩溃并生成核心转储 (Core Dump)，导致设备重启。重启后再次尝试 DNS 解析，陷入无限循环。
【触发条件】DNS_CLIENT-F-SRCADDRFAIL 错误，随后 COREDUMP，然后 SYS-5-RESTART。
【修复步骤】1) 升级 CBS250 固件到修复版本；2) 临时方案：移除默认 NTP/DNS 服务器配置 (no sntp server)，使用 IP 地址替代域名；3) 在交换机上禁用不必要的外部 DNS 解析。`,
			Keywords: "Cisco, CBS250, DNS, DNS_CLIENT, SRCADDRFAIL, COREDUMP, 核心转储, 重启循环, 交换机, NTP",
			Symptoms: "DNS_CLIENT, SRCADDRFAIL, COREDUMP, cisco.com, SYS-5-RESTART, fatal DNS client error, Core dump generated, System rebooting, Result is 2",
		},
		{
			Title:    "Kubernetes 1.29+ ServiceAccount Token 未自动生成导致 InfluxDB 认证失败",
			Category: "网络",
			Content: `【根因】Kubernetes 1.29+ 升级后，ServiceAccount token 的自动生成机制变化，默认 namespace 下的 ServiceAccount 未自动创建 token secret。InfluxDB Cloud 微服务间认证依赖此 token，导致 Authentication service unreachable。
【触发条件】日志出现 "missing service account token" 和 "Authentication service unreachable"。
【修复步骤】1) 手动为 ServiceAccount 创建 token (kubectl create token 或创建 Secret 类型为 kubernetes.io/service-account-token)；2) 检查 kube-controller-manager 的 --service-account-private-key-file 配置；3) 临时方案：直接 mount 手动创建的 token secret 到 Pod。`,
			Keywords: "Kubernetes, InfluxDB, ServiceAccount, token, 认证失败, K8s 1.29, authentication, 微服务, 503",
			Symptoms: "InfluxDB, Authentication service unreachable, missing service account token, Microservices failed to authenticate, Write/Query API failure, service account",
		},
		{
			Title:    "Cisco CBS250 NTP 域名解析失败触发 DNS 客户端缺陷",
			Category: "网络",
			Content: `【根因】Cisco CBS250 交换机默认配置了 time-pnp.cisco.com 作为 SNTP 服务器，当网络环境无法解析该域名时，DNS 客户端的错误处理缺陷被触发，导致设备反复崩溃。
【触发条件】日志显示 "Failed to resolve time-pnp.cisco.com" 和 "Server failure"。
【修复步骤】1) 移除默认 SNTP 服务器配置 "no sntp server time-pnp.cisco.com"；2) 配置可达的 NTP 服务器 IP 地址；3) 确保 DNS 服务器可达或使用 hosts 文件静态映射。`,
			Keywords: "Cisco, CBS250, DNS, NTP, SNTP, time-pnp.cisco.com, 域名解析, lookup failed, Server failure",
			Symptoms: "time-pnp.cisco.com, Failed to resolve, DNS lookup failed, Server failure, SNTP server, CBS250, Removing default SNTP",
		},

		// ────────── 代码缺陷类 ──────────
		{
			Title:    "RAID5 阵列热备盘未自动激活导致双盘离线后阵列崩溃",
			Category: "代码缺陷",
			Content: `【根因】RAID5 阵列中 Physical Disk 2 和 3 先后离线，但因热备盘 (Hot Spare) 配置未正确激活（权限不足或状态异常），阵列无法自动重建，降级后崩溃。Oracle 数据文件因底层存储不可用而损坏。
【触发条件】连续出现 "Physical Disk offline" 且 "Hot spare not activated"。
【修复步骤】1) 检查 RAID 控制器的热备盘配置和权限；2) 手动激活热备盘或更换故障磁盘；3) 从备份恢复 Oracle 数据文件 (system01.dbf)；4) 监控磁盘 SMART 状态，提前更换隐患盘。`,
			Keywords: "RAID5, 热备盘, hot spare, 磁盘离线, Oracle, ORA-01157, 阵列崩溃, disk offline, RAID Controller",
			Symptoms: "RAID_Controller, Physical Disk offline, Hot spare not activated, RAID5 Array failed, ORA-01157, cannot identify, system01.dbf, Physical Disk 2, Physical Disk 3",
		},
		{
			Title:    "TwistLock Defender 干扰 LVM 锁导致 VG-Manager 死循环刷爆磁盘",
			Category: "代码缺陷",
			Content: `【根因】TwistLock Defender (Prisma Cloud) 重启时实时文件扫描干扰了 LVM 的锁文件 (/var/lock/vgmanager/vgmanager.lock) 正常释放。VG-Manager 在获取锁失败后未正确处理重试逻辑，陷入死循环，以每秒 ~26,000 行的速率向日志写入 "Waiting for lock"，10 秒内产生 259,221 行日志，耗尽全部磁盘空间，导致 OVN 数据库损坏。
【触发条件】日志中出现大量重复 "Waiting for lock" 且磁盘使用率迅速飙升到 100%。
【修复步骤】1) 立即停止 TwistLock Defender 服务；2) 手动清理 /var/lock/vgmanager/ 锁文件；3) 清理 VG-Manager 日志释放磁盘空间；4) 修复 OVN DB；5) 在 TwistLock 中加入 /var/lock/vgmanager/ 排除路径；6) VG-Manager 增加重试次数上限和指数退避。`,
			Keywords: "VG-Manager, TwistLock, Prisma Cloud, LVM, 锁文件, 死循环, 磁盘满, infinite loop, lock contention, OVN, OpenShift",
			Symptoms: "VG-Manager, Waiting for lock, vgmanager.lock, disk space exhausted, repeated 259, 100%, OVN DB corruption, TwistLock, lock file, /var/lock, disk usage 100%",
		},

		// ────────── 连接池类 ──────────
		{
			Title:    "Apex One 日志发送配置损坏导致 Vision One 数据上报连接被拒绝",
			Category: "连接池",
			Content: `【根因】Apex One 客户端日志发送配置文件 (Log sending configuration file) 损坏或参数无效，导致向 Trend Vision One 云端数据管道发送日志时连接被拒绝 (Connection refused)。上层控制台因数据缺失显示空白页面。
【触发条件】日志出现 "Log sending configuration file corrupted" 和 "Connection refused"。
【修复步骤】1) 备份损坏的配置文件后删除；2) 从正常节点复制配置模板或重新生成配置；3) 重启 Apex One 日志发送服务；4) 验证 Vision One 控制台数据恢复。`,
			Keywords: "Apex One, Vision One, Connection refused, 配置文件损坏, 日志发送, data pipeline, Trend Micro, 控制台",
			Symptoms: "Apex One, Log sending configuration file corrupted, Connection refused, Trend Vision One, Data ingestion pipeline, blank pages, log sending failure, configuration file",
		},
		{
			Title:    "AKS 自动升级后残留孤立 Node Leases 导致 etcd 超时和 API Server 异常",
			Category: "连接池",
			Content: `【根因】AKS (Azure Kubernetes Service) 集群自动升级节点镜像后，旧节点未正确注销，etcd 中残留孤立 node leases (orphaned leases)。kube-apiserver 在列出租约时因 etcd 中过期条目过多而超时 (context deadline exceeded)，/readyz 探针失败并返回 500。
【触发条件】kube-apiserver 日志出现 "List leases failed: context deadline exceeded" 和 "/readyz probe failed: etcd timeout"。
【修复步骤】1) 使用 etcdctl 手动 revoke 孤儿租约 (etcdctl lease list → etcdctl lease revoke <id>)；2) 重启受影响的 kube-apiserver 实例；3) 确认节点优雅注销流程正常；4) 监控 KubeAPIErrorBudgetBurn 告警恢复。`,
			Keywords: "AKS, etcd, orphaned leases, node leases, API Server, timeout, kube-apiserver, readyz, 500, Azure Kubernetes",
			Symptoms: "AKS, kube-apiserver, readyz, etcd timeout, List leases failed, context deadline exceeded, KubeAPIErrorBudgetBurn, orphaned node leases, node name already exists",
		},

		// ────────── 磁盘类 ──────────
		{
			Title:    "Windows 更新 KB5073722 后 NTFS 索引损坏导致 CHKDSK 无限修复循环",
			Category: "磁盘",
			Content: `【根因】安装 KB5073722 (2026年1月累积更新) 期间 TiWorker.exe 内存泄漏，导致 NTFS 卷 C 上 $I30:$INDEX_ALLOCATION (目录索引分配结构) 损坏。系统检测到损坏后自动触发磁盘检查和修复，但修复过程再次因内存问题失败，形成 CHKDSK 启动循环。
【触发条件】NTFS Event ID 55，$I30:$INDEX_ALLOCATION corruption，随后无限 CHKDSK 循环。
【修复步骤】1) 进入 Windows 恢复环境 (WinRE)；2) 在离线模式下运行 chkdsk C: /f /r；3) 使用 DISM 清理损坏的更新；4) 如 chkdsk 无法修复，使用第三方工具恢复 NTFS 索引。`,
			Keywords: "NTFS, CHKDSK, 磁盘损坏, KB5073722, Event ID 55, $I30, INDEX_ALLOCATION, boot loop, 磁盘检查, Windows Update",
			Symptoms: "NTFS, Event ID 55, Corruption discovered on volume C, $I30, INDEX_ALLOCATION, CHKDSK loop, Disk check required, Rebooting into CHKDSK, Store, Machine",
		},
		{
			Title:    "A-SMGCS 系统 SDP1 服务器缓存模块故障导致服务紧急停机",
			Category: "磁盘",
			Content: `【根因】A-SMGCS (先进场面活动引导与控制系统) 的 SDP1 服务器缓存模块 (Cache Module) 发生硬件或固件故障，write-through 模式写入失败，健康指示灯 (Health indicator LED) 告警。系统自动触发紧急维护流程，停止 SDP1 服务。
【触发条件】日志出现 "Server cache module fault detected" 和 "Health indicator LED abnormal"。
【修复步骤】1) 将 SDP1 服务切换到备用服务器；2) 物理检查 SDP1 缓存模块硬件状态；3) 更换故障缓存模块；4) 在维护窗口内重新上线 SDP1。`,
			Keywords: "SDP1, A-SMGCS, 缓存模块, cache module, 磁盘故障, write-through, health indicator, emergency stop, 服务器",
			Symptoms: "SDP1, Server cache module fault, Health indicator LED abnormal, write-through failure, A-SMGCS, emergency maintenance, cache module",
		},
		{
			Title:    "VG-Manager 日志洪水以每秒 26000 行速率耗尽 233GB 磁盘空间",
			Category: "磁盘",
			Content: `【根因】VG-Manager 进程因无法获取 LVM 锁文件 (vgmanager.lock) 陷入死循环，以每秒约 26,000 行的速率无限制写入日志，最终在极短时间内耗尽全部磁盘空间 (233GB → 100%)。根因通常是安全软件（如 TwistLock/Prisma Cloud Defender）干扰了 LVM 锁机制。
【触发条件】短时间内磁盘使用率从正常飙升至 100%，VG-Manager 日志文件异常增长。
【修复步骤】1) 立即 kill VG-Manager 进程；2) 删除/截断 VG-Manager 日志文件释放空间；3) 手动清理 /var/lock/vgmanager/ 锁文件；4) 在安全软件中添加 LVM 相关路径的白名单。`,
			Keywords: "VG-Manager, 磁盘满, log flood, 日志洪水, LVM, lock, 100%, disk full, 233GB, 26000行",
			Symptoms: "VG-Manager, Waiting for lock, Disk usage 100%, 259,221, exceeds 233GB, Lock to be released, log flood, disk space exhausted",
		},

		// ────────── HTTP 类 ──────────
		{
			Title:    "Trello 后端内存耗尽导致网关 502 Bad Gateway 且自动扩展未触发",
			Category: "HTTP",
			Content: `【根因】Trello 后端服务因内存泄漏或配置不足导致 OOM 崩溃，Nginx/网关无法连接到后端服务，返回 502 Bad Gateway。同时，Auto Scaling 策略仅基于 CPU 指标，未感知到内存压力，扩容未能触发。
【触发条件】连续出现 "502 Bad Gateway: Cannot connect to backend service" 和 "Auto Scaling failed due to memory pressure"。
【修复步骤】1) 扩容后端服务实例内存；2) 配置 HPA 使用 memory 指标或 Prometheus custom metrics；3) 添加 liveness/readiness probe 自动重启 OOM 实例；4) 增加后端实例冗余数量。`,
			Keywords: "502, Bad Gateway, Trello, Auto Scaling, 后端崩溃, 内存, HPA, Nginx, 网关",
			Symptoms: "Trello, 502 Bad Gateway, Cannot connect to backend, Upstream server temporarily unavailable, Auto Scaling failed, memory pressure",
		},
		{
			Title:    "AKS API Server /readyz 返回 500 但 Resource Health 仍显示 Available",
			Category: "HTTP",
			Content: `【根因】AKS 集群自动升级后，6 个 API Server 实例中 4 个的 /readyz 就绪探针失败（内部 etcd 通信超时或组件异常），返回 HTTP 500。Azure 负载均衡器的健康检查未正确排除异常实例，Resource Health 仪表盘仍显示 "Available"，掩盖了实际故障。
【触发条件】Prometheus KubeAPIErrorBudgetBurn 告警触发，/readyz 返回 500，但 Resource Health = Available。
【修复步骤】1) 检查异常 API Server 实例的 etcd 连接；2) 手动 cordon 异常节点；3) 验证负载均衡器健康探针配置；4) 向 Azure 支持确认 Resource Health 监控逻辑。`,
			Keywords: "500, Internal Server Error, AKS, /readyz, API Server, KubeAPIErrorBudgetBurn, Prometheus, 就绪探针, Resource Health",
			Symptoms: "AKS, /readyz, returning 500, API server instances, KubeAPIErrorBudgetBurn, Resource Health remains Available, Prometheus",
		},
		{
			Title:    "InfluxDB Cloud 认证服务不可用导致所有查询返回 503",
			Category: "HTTP",
			Content: `【根因】InfluxDB Cloud 的认证服务 (Authentication Backend) 因依赖的上游服务（如 K8s ServiceAccount token 服务）故障而不可用。所有查询 API 在认证环节失败，返回 503 Service Unavailable。影响范围涉及整个 EU-Central-1 区域。
【触发条件】日志出现 "503 Service Unavailable: Authentication backend unreachable" 和 "All queries failing"。
【修复步骤】1) 检查认证服务的依赖链：K8s ServiceAccount → token → auth service；2) 重启认证服务 Pod；3) 验证 ServiceAccount token 卷挂载正确；4) 考虑认证服务的高可用架构。`,
			Keywords: "503, Service Unavailable, InfluxDB, 认证, authentication, EU-Central-1, query, 全部查询失败",
			Symptoms: "InfluxDB, 503 Service Unavailable, Authentication backend unreachable, All queries failing, EU-Central-1, Query service unavailable, authentication",
		},

		// ────────── 权限类 ──────────
		{
			Title:    "RAID 控制器热备盘激活因权限不足失败导致阵列降级后崩溃",
			Category: "权限",
			Content: `【根因】RAID 控制器热备盘 (Hot Spare) 在有磁盘离线时尝试自动激活，但因控制器配置中权限不足 (Insufficient privileges to enable hot spare feature) 而失败。阵列在降级状态下运行，后续又有磁盘离线时无热备盘可用，阵列崩溃。Oracle 数据库因存储不可用报 Permission denied。
【触发条件】日志同时出现 "Hot spare disk activation failed" 和 "Insufficient privileges"。
【修复步骤】1) 以管理员权限登录 RAID 控制器管理界面；2) 检查并更新热备盘激活策略和权限配置；3) 手动激活热备盘；4) 替换故障磁盘并重建阵列。`,
			Keywords: "RAID, 热备盘, hot spare, 权限不足, Permission denied, activation failed, insufficient privileges, Oracle, 阵列降级",
			Symptoms: "RAID_Controller, Hot spare disk activation failed, Insufficient privileges, enable hot spare, Permission denied on datafile, Database mount failed",
		},
		{
			Title:    "Kubernetes 未自动创建 ServiceAccount Token 导致 InfluxDB 认证权限被拒绝",
			Category: "权限",
			Content: `【根因】Kubernetes 集群升级后（尤其是到 1.24+ 版本），不再自动为 ServiceAccount 创建长期有效的 token Secret。InfluxDB 微服务在尝试认证时找不到有效的 ServiceAccount token，认证失败导致权限被拒绝 (authorization failed / permission denied)。
【触发条件】"Failed to authenticate service account" 和 "Missing service account token for default namespace"。
【修复步骤】1) 手动创建 ServiceAccount token Secret；2) 在 Deployment/Pod spec 中显式挂载 token 卷；3) 如使用短期 token，确保 token 刷新机制正常。`,
			Keywords: "Kubernetes, ServiceAccount, token, 权限, authentication, Permission denied, RBAC, InfluxDB, authorization",
			Symptoms: "InfluxDB, Failed to authenticate service account, Missing service account token, Write API authorization failed, Permission denied, default namespace, service account",
		},
		{
			Title:    "Linux OOM Killer 权限机制强制终止内存压力下的用户进程",
			Category: "权限",
			Content: `【根因】Linux 内核在系统内存极度不足时，OOM Killer 会根据进程的 oom_score（由内存占用、运行时间等因素计算）选择分数最高的进程强制发送 SIGKILL (signal 9)。这是内核级别的资源管理机制，不受进程自身权限控制。
【触发条件】系统日志出现 "invoked oom-killer"，随后目标进程被 "Killed" (exit code=KILL, status=9/KILL)。
【修复步骤】1) 调整关键进程的 oom_score_adj 为负值降低被杀概率；2) 增加系统内存或配置 swap；3) 为关键服务设置 cgroup 内存限制和 OOM 处理策略；4) 使用 systemd 的 OOMPolicy=continue 让 systemd 处理而非内核直接 kill。`,
			Keywords: "OOM Killer, oom-killer, KILL, signal 9, systemd, oom_score_adj, Linux内核, 内存压力",
			Symptoms: "oom-killer, invoked oom-killer, gfp_mask, code=killed, status=9/KILL, oom-kill, Main process exited, Failed with result, vector.service",
		},

		// ────────── 数据库类 ──────────
		{
			Title:    "RAID5 阵列崩溃导致 Oracle 系统表空间文件损坏无法启动",
			Category: "数据库",
			Content: `【根因】底层 RAID5 阵列因磁盘离线+热备盘未激活而崩溃，Oracle 数据库的系统表空间文件 /data/orcl/system01.dbf 因底层存储不可用而损坏。数据库实例无法识别或锁定数据文件 (ORA-01157)，启动失败。
【触发条件】Oracle alert log 出现 ORA-01157 (cannot identify/lock data file) 和 ORA-01110 (data file path)。
【修复步骤】1) 先修复底层存储：替换故障磁盘、重建 RAID 阵列；2) 从备份恢复 system01.dbf；3) 使用 RMAN 进行恢复 (recover database)；4) 如果备份不可用，尝试使用 _allow_resetlogs_corruption 强制打开数据库后导出数据。`,
			Keywords: "Oracle, ORA-01157, ORA-01110, system01.dbf, RAID5, 数据库损坏, 表空间, recovery, RMAN",
			Symptoms: "ORA-01157, cannot identify, ORA-01110, system01.dbf, data file 1, Database instance terminated, Recovery needed, DBWR trace file, Oracle DB",
		},
		{
			Title:    "磁盘写满导致 OVN Southbound 数据库损坏和 OpenShift 集群网络瘫痪",
			Category: "数据库",
			Content: `【根因】节点磁盘被 VG-Manager 日志洪水写满 (100% disk usage)，OVN 的 Southbound 数据库 (OVN_Southbound DB) 因无法写入而损坏。OpenShift SDN (软件定义网络) 依赖 OVN DB 存储网络拓扑信息，数据库损坏导致集群网络降级 (Cluster networking degraded)。
【触发条件】"OVN DB corruption" + "Cluster networking degraded" + 磁盘使用率 100%。
【修复步骤】1) 清理日志释放磁盘空间；2) 停止 ovn-controller；3) 从备份恢复 OVN Southbound DB 或重建；4) 重启 ovn-kubernetes master；5) 验证节点网络恢复。`,
			Keywords: "OVN, OpenShift, 数据库损坏, Southbound DB, SDN, 集群网络, disk full, corruption, OVN DB",
			Symptoms: "OVN, Database corruption detected, OVN_Southbound DB, Unable to read, Cluster networking degraded, OpenShift, disk space",
		},

		// ────────── CPU 类 ──────────
		{
			Title:    "B2ts_v2 VM CPU 积分耗尽导致 HAProxy 性能受限和连接超时",
			Category: "CPU",
			Content: `【根因】B2ts_v2 虚拟机（Azure B 系列突发性能型 VM）的 CPU 积分 (CPU Credits) 在高负载期间耗尽，CPU 性能被严重限制 (throttled)。HAProxy 请求处理变慢，rsyslog 因磁盘 I/O 阻塞导致 CPU 大量等待 I/O。
【触发条件】"CPU credits exhausted. Performance throttled" + "High CPU usage detected: 100%"。
【修复步骤】1) 监控 CPU 积分余额，在耗尽前扩容；2) 将 VM 类型升级为非突发性能型（如 D 系列）；3) 优化 HAProxy 配置减少 CPU 消耗；4) 检查 rsyslog 写入目标和缓冲区配置。`,
			Keywords: "CPU, B2ts_v2, CPU credits, throttled, HAProxy, rsyslog, I/O blocked, 性能受限, 积分耗尽",
			Symptoms: "B2ts_v2, CPU credits exhausted, Performance throttled, HAProxy, High CPU usage detected, 100%, rsyslog, Disk I/O blocked, CPU waiting for I/O",
		},
		{
			Title:    "Cisco CBS250 DNS 解析死循环导致 CPU 致命异常并重启",
			Category: "CPU",
			Content: `【根因】Cisco CBS250 交换机在 DNS 解析外部域名（如 time-pnp.cisco.com）时陷入死循环。DNS 解析任务 (DNSC task) 占用 100% CPU，最终触发致命 CPU 异常 (fatal CPU exception)，交换机自动重启。
【触发条件】"High CPU utilization during DNS resolution loop" + "Core dump triggered by DNSC task" + "fatal CPU exception"。
【修复步骤】1) 升级固件版本修复 DNS 处理缺陷；2) 禁用对外部域名的 DNS 解析任务；3) 使用静态 IP 替代域名配置；4) 配置 DNS 服务器为可达地址。`,
			Keywords: "CPU, Cisco, CBS250, DNS, 死循环, loop, core dump, DNSC task, fatal CPU exception, 重启",
			Symptoms: "CBS250, High CPU utilization during DNS resolution, Core dump triggered by DNSC, fatal CPU exception, Switch rebooted, DNS resolution loop",
		},

		// ────────── 配置类 ──────────
		{
			Title:    "Apex One 日志配置损坏导致参数解析失败和远程上报中断",
			Category: "配置",
			Content: `【根因】Apex One 日志发送模块的配置文件因磁盘写入异常或软件 bug 而损坏。程序尝试解析配置时遇到 Invalid parameter 错误，无法正确读取日志发送目标配置，回退到本地日志模式 (Defaulting to local logging only)。
【触发条件】"Log configuration mismatch" + "Failed to parse log sending config: Invalid parameter"。
【修复步骤】1) 从 Apex One 管理控制台重新下发日志配置；2) 删除客户端端的损坏配置文件，让程序重建默认配置；3) 检查磁盘是否有写入错误；4) 升级 Apex One 客户端到最新版本。`,
			Keywords: "Apex One, 配置损坏, log configuration, 参数无效, parse error, invalid parameter, 配置文件, 日志上报",
			Symptoms: "Apex One, Log configuration mismatch, Failed to parse log sending config, Invalid parameter, Defaulting to local logging only, configuration file",
		},
		{
			Title:    "AKS 自动升级后旧节点未注销导致 etcd 残留孤立租约",
			Category: "配置",
			Content: `【根因】AKS 集群在自动节点镜像升级 (Automatic node image upgrade) 过程中，旧节点被新节点替换时未正确注销 (deregister)。etcd 中残留旧节点的 node leases，导致 etcd 数据库中存在大量过期条目，影响 API Server 性能。
【触发条件】"Failed to register node: node name already exists" + "Orphaned node leases detected in etcd"。
【修复步骤】1) 使用 etcdctl 列出并 revoke 孤儿租约；2) 手动清理未正确注销的节点对象 (kubectl delete node <name>)；3) 联系 Azure 支持检查升级流程；4) 在升级前确保所有节点状态正常。`,
			Keywords: "AKS, etcd, orphaned leases, 节点注册, node upgrade, kubelet, register node, 升级, 配置",
			Symptoms: "AKS, automatic node image upgrade, Failed to register node, node name already exists, Orphaned node leases detected in etcd, kubelet",
		},
	}

	// 批量插入
	if err := database.DB.Create(&entries).Error; err != nil {
		log.Fatalf("插入知识库失败: %v", err)
	}

	// 验证
	var count int64
	database.DB.Model(&model.KnowledgeBase{}).Count(&count)
	fmt.Printf("✅ 知识库种子数据插入完成，共 %d 条\n", count)

	// 按分类统计
	var categories []struct {
		Category string
		Count    int64
	}
	database.DB.Model(&model.KnowledgeBase{}).
		Select("category, count(*) as count").
		Group("category").
		Find(&categories)
	fmt.Println("\n分类统计:")
	for _, c := range categories {
		fmt.Printf("  %s: %d 条\n", c.Category, c.Count)
	}

	os.Exit(0)
}
