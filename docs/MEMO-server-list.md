mactl server list で、サーバーが複数のネットワークに接続している場合、
行を変えて、IPアドレスと仮想ネットワークの名前を表示したい。

現在の表示スタイル
ubuntu@ws1:~$ mactl server list
  No  Server-ID   Server-Name           Status        CPU  RAM(MB)   Node          IP-Address       Network        
   1  3f738       test-server-40        RUNNING       1    2048      marmot1       192.168.100.2    test-net-4     
   2  592a2       test-server-41        RUNNING       1    2048      marmot2       192.168.100.3    test-net-4 
   3  396a3       test-server-42        RUNNING       1    2048      marmot3       192.168.100.4    test-net-4    


変更したい表示スタイル
ubuntu@ws1:~$ mactl server list
  No  Server-ID   Server-Name           Status        CPU  RAM(MB)   Node          IP-Address       Network        
   1  3f738       test-server-40        RUNNING       1    2048      marmot1       192.168.100.2    test-net-4
                                                                                   192.168.1.71     host-bridge
                                                                                   N/A              default
   2  592a2       test-server-41        RUNNING       1    2048      marmot2       192.168.100.3    test-net-4 
                                                                                   192.168.1.72     host-bridge
                                                                                   N/A              default
   3  396a3       test-server-42        RUNNING       1    2048      marmot3       192.168.100.4    test-net-4    
                                                                                   192.168.1.72     host-bridge
                                                                                   N/A              default

