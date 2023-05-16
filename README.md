# About

systemd-remote is a tool to remotely configure systemd unit files using grpc.

While some effort has been made to generalize, the current supported functionality is centered around our use-cases and will be expanded as our need grows.

Currently, we depend on the follow 2 workflows which are supported:
  - A single **.service** unit file that acts as a persistent background deamon.
  - A **.service** unit file that acts as a recurring job and an accompanying **.timer** file to control its reccurence.

Note that due to the sensitivity of **systemd**, systemd-remote will always expect to be talked to via mtls with client certifcate authentication, even locally.

# Grpc Workflow

systemd-remote does the server implementation of the following protocol: https://github.com/Ferlab-Ste-Justine/etcd-sdk/blob/main/keypb/api.proto#L42

## Supported Files

It expects unit updates to be pushed to it.

It supports 3 kind of files:
- Service unit files (files ending with the **.service** extention)
- Timer unit files (files ending with the **.timer** extention)
- A **units.yml** configuration file

## Persistent Units

For terminology's sake, we use the term **persistent service** to refer to a timer unit or a service unit that:
- Is not managed by the **units.yml** file
- Is managed by the **units.yml** file and is not a job triggered by a timer.

## units.yml

The **units.yml** file has the following format:

```
<unit file name>:
  name: Unit file name
  job: Boolean indication wether a ".service" file is a job activated by a ".timer" file or a persistent service. Applies only to files of type ".service" 
  on: Whether the unit is started/enabled or stopped/disabled. Applies only to persistent services.
...
```

The behavior is as follows:
- If a unit entry is **deleted** and it is a **persistent service** that is **on**, it will be **stopped** and **disabled**
- If a unit entry is **added** and it is a **persistent service** that is **on**, it will be **started** and **enabled**
- If a unit entry's **on** status is modified, the unit will be either **stopped** and **disabled** or **started** and **enabled** to reflect that. 

Additionally, the following rules are enforced:
- The **name** entry cannot be empty.
- The **job** property can only be set to **true** for units of type **.service**.
- The **job** property of a unit cannot be changed once set. If you wish you change it, you will have to delete the unit and recreate it.
- A unit that has its **job** property set to **true** cannot also have its **on** property set to **true** (you should activate or deactivate its accompanying **.timer** unit instead).

## Changing Unit Files

Whenever a unit file is added, updated or deleted, systemd-remote will always work in the **/etc/systemd/system** directory to enact the change.

Changing unit files will be handled as such:
  - Insert/Update:
    - Create/Update the unit file
    - Reload the systemd daemon
    - If the unit is a persistent service and active, it is restarted
  - Deletion:
    - If the unit is active, it is stopped and disabled
    - Delete the unit file
    - Reload the systemd daemon

# Configuration

The path of configuration file for systemd-remote can be defined with the **SYSTEMD_REMOTE_CONFIG_FILE** environment variable. It defaults to a file named **config.yml** in the running directory of the process.

The configuration is in yaml format and takes the following parameters:
- **units_config_path**: String value indicating the path where systemd-remote should persist changes to its units state (ie, **units.yml**). If no file is found at that path, an empty one will be created.
- **log_level**: Only logs at this level or importance or greater will be shown. Possible values, in order of importance from least to most, are: debug, info, warn, error
- **server**: Parameters for the grpc server. It takes the properties shown below.
  - **port**: Port to listen on
  - **address**: Address to bind on to listen for incoming traffic
  - **tls**: Parameters for mtls. It takes the properties shown below.
    - **ca_cert**: Path to a CA certificate file that will be used to authentify clients
    - **server_cert**: Path to a server certificate file that will be presented to connecting clients
    - **server_key**: Path to a private server key file that will be used to authentify the server with the certificate it presents to the clients.
