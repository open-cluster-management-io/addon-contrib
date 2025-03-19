FROM quay.io/jupyter/scipy-notebook:2024-12-23

# Switch to root for permission fixes
USER root

# Grant group 0 full permissions to /home/jovyan
RUN chgrp -R 0 /home/jovyan && \
  chmod -R g+rwX /home/jovyan

# Or use the built-in fix-permissions script if available
# RUN fix-permissions /home/jovyan

# Switch back to the regular notebook user (often 1000)
USER 1000