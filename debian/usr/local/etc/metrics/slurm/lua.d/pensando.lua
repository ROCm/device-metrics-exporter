-- ===========================================================================
-- SPANK plugin to demonstrate when and where and by whom the SPANK functions
-- are called.
-- ===========================================================================

--
-- includes
--

local posix = require("posix")
local zmq = require"lzmq"
local socket = require 'socket'
local pb = require "pb"
local pbio = require "pb.io"
local protoc = require "protoc"
--
-- constants
--

myname = "AMD-PensandoSlurmPlugin"


--
-- functions
--

function gethostname()
    local f = io.popen ("/bin/hostname")
    local hostname = f:read("*a") or ""
    f:close()
    hostname = string.gsub(hostname, "\n$", "")
    return hostname
end

function getuid()
    local f = io.popen ("/bin/id -un")
    local uid = f:read("*a") or ""
    f:close()
    uid = string.gsub(uid, "\n$", "")
    return uid
end

function getgid()
    local f = io.popen ("/bin/id -gn")
    local gid = f:read("*a") or ""
    f:close()
    gid = string.gsub(gid, "\n$", "")
    return gid
end

function getdevicecgroup()
    local f = io.popen ("grep devices /proc/$$/cgroup  | cut -d: -f3")
    local d_cg = f:read("*a") or ""
    f:close()
    d_cg = string.gsub(d_cg, "\n$", "")
    return d_cg
end

local function printTable( t )
   local out = ""
   local function sprint(str)
       out = out .. str
   end
 
    local printTable_cache = {}
    local function sub_printTable( t, indent )
        if ( printTable_cache[tostring(t)] ) then
            sprint( indent .. "*" .. tostring(t) )
        else
            printTable_cache[tostring(t)] = true
            if ( type( t ) == "table" ) then
                for pos,val in pairs( t ) do
                    if ( type(val) == "table" ) then
                        sprint( indent .. "[" .. pos .. "] => " .. tostring( t ).. " {" )
                        sub_printTable( val, indent .. string.rep( " ", string.len(pos)+8 ) )
                        sprint( indent .. string.rep( " ", string.len(pos)+6 ) .. "}" )
                    elseif ( type(val) == "string" ) then
                        sprint( indent .. "[" .. pos .. '] => "' .. val .. '"' )
                    else
                        sprint( indent .. "[" .. pos .. "] => " .. tostring(val) )
                    end
                end
            else
                sprint( indent..tostring(t) )
            end
        end
    end
 
    if ( type(t) == "table" ) then
        sprint( tostring(t) .. " {" )
        sub_printTable( t, "  " )
        sprint( "}" )
    else
        sub_printTable( t, "  " )
    end
    return out
end

function display_msgold(spank, caller)
    local context = spank.context
    local hostname = gethostname()
    local uid = getuid()
    local gid = getgid()
    local device_cgroup = getdevicecgroup()

--     local out = printTable(spank)
    local cudadev = spank:getenv("CUDA_VISIBLE_DEVICES")
    local rocrdev = spank:getenv("ROCR_VISIBLE_DEVICES")
    local gpugrp = spank:getenv("GPUGROUPID")
    local jobenv = spank:get_item("S_JOB_ENV")
    local out = printTable(jobenv)
    if cudadev == nil then cudadev = "NOTSET" end
    if rocrdev == nil then rocrdev = "NOTSET" end
    if gpugrp == nil then gpugrp = "NOTSET" end
--     SPANK.log_info("%s: ctx:%s host:%s caller:%s uid:%s gid:%s device_cgroup:%s fullMsg: %s" , myname, context, hostname, caller, uid, gid, device_cgroup, out)
    -- SPANK.log_info("%s: ctx:%s host:%s caller:%s uid:%s gid:%s device_cgroup:%s CUDA_DEVS: %s ROCR_DEVs= %s GPU_GROUPID: %s JOb ENV: %s " , myname, context, hostname, caller, uid, gid, device_cgroup, cudadev, rocrdev, gpugrp, out)
--     SPANK.log_info("%s: ctx:%s host:%s caller:%s uid:%s gid:%s device_cgroup:%s CUDA_DEVS: %s ROCR_DEVs= %s GPU_GROUPID: %s " , myname, context, hostname, caller, uid, gid, device_cgroup, cudadev, rocrdev, gpugrp)
     SPANK.log_info("%s: ctx:%s host:%s caller:%s uid:%s gid:%s device_cgroup:%s" , myname, context, hostname, caller, uid, gid, device_cgroup)
    return 0
end

function display_msg(spank, caller)
    local context = spank.context
    local hostname = gethostname()
    local uid = getuid()
    local gid = getgid()
    local device_cgroup = getdevicecgroup()

    SPANK.log_info("%s: ctx:%s host:%s caller:%s uid:%s gid:%s device_cgroup:%s" , myname, context, hostname, caller, uid, gid, device_cgroup)
    return 0
end

function mysplit (inputstr, sep)
        if sep == nil then
                sep = "%s"
        end
        local t={}
	if  inputstr == nil then return t end
        for str in string.gmatch(inputstr, "([^"..sep.."]+)") do
                table.insert(t, str)
        end
        return t
end

function GetSpankData (spank)
    local cudadev = spank:getenv("CUDA_VISIBLE_DEVICES")
    if cudadev ~= nil then cudadev = "" end
    local spankData = {
	JobID = spank:get_item("S_JOB_ID"),
	JobGID = spank:get_item("S_JOB_GID"),
	JobUID = spank:get_item("S_JOB_UID"),
	JobStepID = spank:get_item("S_JOB_STEPID"),
	NNodes = spank:get_item("S_JOB_NNODES"),
	NodeID = spank:get_item("S_JOB_NODEID"),
	NCPus = spank:get_item("S_JOB_NCPUS"),
	TaskID = spank:get_item("S_TASK_ID"),
	TaskPID = spank:get_item("S_TASK_PID"),
	AllocCores = mysplit(spank:get_item("S_JOB_ALLOC_CORES"), " "),
	AllocGPUs = mysplit(cudadev, ",")
    }
    return spankData
end

function GetEnumValue (type, val)
    if val == "task_init"
    then return 0
    elseif val == "task_exit"
    then return 1
    elseif val == "epilog"
    then return 2
    end
end

function send_msg(spank, stage)
    protoc:load(pbio.read( "/usr/local/etc/slurm/plugin.proto", "/usr/local/etc/slurm/plugin.proto"))
    --local device_cgroup = getdevicecgroup()

    StagesEnum = pb.Stages
    local msg = {
        Hostname = gethostname(),
        Context = spank.context,
        UID = getuid(),
        GID = getgid(),
        Cgroup = getdevicecgroup(),
        Type = GetEnumValue(StagesEnum, stage),
	    SData = GetSpankData(spank)
    }

    if msg.SData == nil 
    then
         context:term()
         return
    end

    bytes = pb.encode("plugin.Notification", msg)
    local context = zmq.context()

    local requester, err = context:socket{zmq.PUSH, connect = "tcp://localhost:6601"}

    requester:send(bytes)
    socket.sleep(0.1)
    context:term()
end


--
-- SPANK functions
-- cf. https://slurm.schedmd.com/spank.html
--

function slurm_spank_init (spank)
    display_msg(spank, "slurm_spank_init")
--     send_msg(spank, "spank_init")
    return 0
end

function slurm_spank_slurmd_init (spank)
    display_msg(spank, "slurm_spank_slurmd_init")
--     send_msg(spank, "slurmd_init")
    return 0
end

function slurm_spank_job_prolog (spank)
    display_msg(spank, "slurm_spank_job_prolog")
--     if not spank:setenv("GPUGROUPID", "123", 0) then
--         SPANK.log_info("Failed to set GPU_GROUPID")
--     end
--     send_msg(spank, "post_opt")
    return 0
end

function slurm_spank_init_post_opt (spank)
    display_msg(spank, "slurm_spank_init_post_opt")
--     send_msg(spank, "post_opt")
    return 0
end

function slurm_spank_local_user_init (spank)
    display_msg(spank, "slurm_spank_local_user_init")
--     send_msg(spank, "local_user_init")
    return 0
end

function slurm_spank_user_init (spank)
    display_msg(spank, "slurm_spank_user_init")
--     send_msg(spank, "spank_user_init")
    return 0
end

function slurm_spank_task_init_privileged (spank)
    display_msg(spank, "slurm_spank_task_init_privileged")
--     send_msg(spank, "task_priv_init")
    return 0
end

function slurm_spank_task_init (spank)
    if not spank:setenv("GPUGROUPID", "123", 1) then
        SPANK.log_info("Failed to set GPU_GROUPID")
    end
    display_msg(spank, "slurm_spank_task_init")
    send_msg(spank, "task_init")
    return 0
end

function slurm_spank_task_post_fork (spank)
    display_msg(spank, "slurm_spank_task_post_fork")
--     send_msg(spank, "task_post_fork")
    return 0
end

function slurm_spank_task_exit (spank)
    display_msg(spank, "slurm_spank_task_exit")
    send_msg(spank, "task_exit")
    return 0
end

function slurm_spank_exit (spank)
    display_msg(spank, "slurm_spank_exit")
--     send_msg(spank, "spank_exit")
    return 0
end

function slurm_spank_job_epilog (spank)
    display_msg(spank, "slurm_spank_job_epilog")
    send_msg(spank, "epilog")
    return 0
end

function slurm_spank_slurmd_exit (spank)
    display_msg(spank, "slurm_spank_slurmd_exit")
    -- send_msg(spank, "slurm_exit")
    return 0
end
