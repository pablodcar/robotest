#
# Virtual Machine node
#

resource "azurerm_public_ip" "node" {
  count                        = "${var.nodes}"
  name                         = "node-${count.index}"
  location                     = "${var.location}"
  resource_group_name          = "${azurerm_resource_group.robotest.name}"
  public_ip_address_allocation = "dynamic"
}

resource "azurerm_network_interface" "node" {
  count                = "${var.nodes}"
  name                 = "node-${count.index}"
  location             = "${var.location}"
  resource_group_name  = "${azurerm_resource_group.robotest.name}"
  enable_ip_forwarding = "true"
  network_security_group_id = "${azurerm_network_security_group.robotest.id}"

  ip_configuration {
    name                          = "ipconfig-${count.index}"
    subnet_id                     = "${azurerm_subnet.robotest_a.id}"
    private_ip_address_allocation = "dynamic"
    public_ip_address_id          = "${azurerm_public_ip.node.*.id[count.index]}"
  }
}

resource "azurerm_virtual_machine" "node" {
  count                 = "${var.nodes}"
  name                  = "node-${count.index}"
  location              = "${var.location}"
  resource_group_name   = "${azurerm_resource_group.robotest.name}"
  network_interface_ids = ["${azurerm_network_interface.node.*.id[count.index]}"]
  vm_size               = "${var.vm_type}"

  delete_os_disk_on_termination    = "true"
  delete_data_disks_on_termination = "true"

  storage_image_reference {
    publisher = "${lookup(var.os_publisher, var.os)}"
    offer     = "${lookup(var.os_offer,     var.os)}"
    sku       = "${lookup(var.os_sku,       var.os)}"
    version   = "${lookup(var.os_version,   var.os)}"
  }

  storage_os_disk {
    name                = "node-os-${count.index}"
    caching             = "ReadWrite"
    create_option       = "FromImage"
    managed_disk_type   = "Premium_LRS"
    disk_size_gb        = "64"
  }

  os_profile {
    custom_data    = "${file("./bootstrap/${var.os}.sh")}"
    computer_name  = "node-${count.index}"
    # REQUIRED ...
    admin_username = "${var.ssh_user}"
    admin_password = "Password1234!"
  }

  os_profile_linux_config {
    disable_password_authentication = true    
    ssh_keys = {
        path = "/home/${var.ssh_user}/.ssh/authorized_keys"
        key_data = "${file("${var.ssh_authorized_keys_path}")}"
    }
  }

  storage_data_disk {
    name              = "node-etcd-${count.index}"
    managed_disk_type = "Premium_LRS"
    create_option     = "Empty"
    lun               = 0
    disk_size_gb      = "64"
  }

  storage_data_disk {
    name              = "node-docker-${count.index}"
    managed_disk_type = "Premium_LRS"
    create_option     = "Empty"
    lun               = 1
    disk_size_gb      = "64"
  }

}
