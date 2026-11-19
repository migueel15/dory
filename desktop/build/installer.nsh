; Windows asks before a program listens on the network. The mobile app and
; the other nodes reach the core on the mesh port, so the installer allows it
; with the administrator consent, only when the rule is missing: updates run
; the installer again and must not ask each time.
!macro customInstall
  nsExec::ExecToStack 'netsh advfirewall firewall show rule name="Dory"'
  Pop $0
  Pop $1
  ${if} $0 != 0
    ExecShellWait "runas" "netsh" 'advfirewall firewall add rule name="Dory" dir=in action=allow protocol=TCP localport=7710 profile=any' SW_HIDE
  ${endif}
!macroend

!macro customUnInstall
  ${ifNot} ${isUpdated}
    ExecShellWait "runas" "netsh" 'advfirewall firewall delete rule name="Dory"' SW_HIDE
  ${endif}
!macroend
