# mactl get srv の表示

現状は、以下の表示ですが、見やすくするために、改善したい。

```console
$ mactl get srv
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE   
----             ----          ------        ---  -------  ----------       -------          ---   
web-1            marmot2       RUNNING       1    1024     172.16.8.3       app-net          13m   
                                                           N/A              default                
                                                           192.168.1.64     host-bridge            
web-2            marmot3       RUNNING       1    1024     172.16.8.2       app-net          13m   
                                                           N/A              default                
web-3            marmot1       RUNNING       1    1024     172.16.8.4       app-net          13m   
                                                           N/A              default         
```

`mactl get srv` とした時は、IP-ADDRESSが無い行が２行目に来るときは、省略する。
しかし、１行目に来る時は、省略しない。

```console
$ mactl get srv
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE   
----             ----          ------        ---  -------  ----------       -------          ---   
web-1            marmot2       RUNNING       1    1024     172.16.8.3       app-net          13m   
                                                           192.168.1.64     host-bridge            
web-2            marmot3       RUNNING       1    1024     172.16.8.2       app-net          13m   
web-3            marmot1       RUNNING       1    1024     172.16.8.4       app-net          13m   
```

オプション `-a` を付加することで、全行を表示する。


```console
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE   
----             ----          ------        ---  -------  ----------       -------          ---   
web-1            marmot2       RUNNING       1    1024     172.16.8.3       app-net          20m   
                                                           N/A              default                
                                                           192.168.1.64     host-bridge            
web-2            marmot3       RUNNING       1    1024     172.16.8.2       app-net          20m   
                                                           N/A              default                
web-3            marmot1       RUNNING       1    1024     172.16.8.4       app-net          20m   
                                                           N/A              default                
lb-e11ef         marmot1       RUNNING       1    2048     192.168.1.70     host-bridge      7m    
                                                           172.16.8.1       app-net     
```

