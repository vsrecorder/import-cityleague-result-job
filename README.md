# import-cityleague-result-job

```
make build
sudo cp systemd/* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now import-cityleague-result-job_enqueue.timer
sudo systemctl enable --now import-cityleague-result-job_dequeue.timer
sudo systemctl restart import-cityleague-result-job_enqueue.timer
sudo systemctl restart import-cityleague-result-job_dequeue.timer
```

```
systemctl list-timers
```

```
sudo systemctl stop import-cityleague-result-job_enqueue.timer
sudo systemctl stop import-cityleague-result-job_dequeue.timer
```

```
sudo systemctl start import-cityleague-result-job_enqueue.timer
sudo systemctl start import-cityleague-result-job_dequeue.timer
```

```
sudo systemctl status import-cityleague-result-job_enqueue.timer
sudo systemctl status import-cityleague-result-job_enqueue.service
sudo systemctl status import-cityleague-result-job_dequeue.timer
sudo systemctl status import-cityleague-result-job_dequeue.service
```

```
journalctl -eu import-cityleague-result-job_enqueue
journalctl -eu import-cityleague-result-job_dequeue
```
